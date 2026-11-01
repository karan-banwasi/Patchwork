package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	githubRepo         = "karan-banwasi/Patchwork"
	defaultGitHubAPI   = "https://api.github.com"
	maxManifestSize    = 1 << 20 // 1 MiB
	metadataTimeout    = 30 * time.Second
	binaryDownloadTime = 10 * time.Minute
)

type selfUpdateOptions struct {
	force              bool
	skipAttestation    bool
	requireAttestation bool
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type ghAttestationsResponse struct {
	Attestations []struct {
		Bundle json.RawMessage `json:"bundle"`
	} `json:"attestations"`
}

// isNewerVersion compares two semantic version tags (e.g. "v1.0.5" vs "v1.0.4").
func isNewerVersion(current, latest string) bool {
	if current == "" || current == "dev" {
		return true
	}
	cParts := parseSemverParts(current)
	lParts := parseSemverParts(latest)
	for i := 0; i < 3; i++ {
		if lParts[i] > cParts[i] {
			return true
		}
		if lParts[i] < cParts[i] {
			return false
		}
	}
	return false
}

func parseSemverParts(v string) [3]int {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var res [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		if n, err := strconv.Atoi(parts[i]); err == nil {
			res[i] = n
		}
	}
	return res
}

// extractExpectedSHA256 parses KaranBanwasi.Patchwork.installer.yaml content and returns
// the expected SHA256 hash (uppercase hex) for the given architecture ("x64" or "arm64").
func extractExpectedSHA256(installerYAML, arch string) string {
	lines := strings.Split(installerYAML, "\n")
	inMatchingArch := false
	archPattern := regexp.MustCompile(`(?i)^\s*-?\s*Architecture:\s*(\S+)`)
	shaPattern := regexp.MustCompile(`(?i)^\s*InstallerSha256:\s*([A-Fa-f0-9]{64})`)

	for _, line := range lines {
		if m := archPattern.FindStringSubmatch(line); len(m) == 2 {
			inMatchingArch = strings.EqualFold(m[1], arch)
			continue
		}
		if inMatchingArch {
			if m := shaPattern.FindStringSubmatch(line); len(m) == 2 {
				return strings.ToUpper(m[1])
			}
		}
	}
	return ""
}

// detectManagedInstall checks whether the executable path belongs to a package manager (Winget or Scoop).
func detectManagedInstall(exePath string) error {
	normalized := strings.ReplaceAll(strings.ToLower(exePath), "/", `\`)
	switch {
	case strings.Contains(normalized, `\microsoft\winget\packages\`) || strings.Contains(normalized, `\microsoft\winget\links\`):
		return fmt.Errorf("Patchwork was installed with winget; run: winget upgrade KaranBanwasi.Patchwork (or pass --force to override)")
	case strings.Contains(normalized, `\scoop\apps\`) || strings.Contains(normalized, `\scoop\shims\`):
		return fmt.Errorf("Patchwork was installed with Scoop; run: scoop update patchwork (or pass --force to override)")
	}
	return nil
}

// cleanupStaleUpdateFiles removes leftover temporary binaries from interrupted self-updates.
func cleanupStaleUpdateFiles(exeDir, exePath string) {
	_ = os.Remove(exePath + ".old")
	matches, err := filepath.Glob(filepath.Join(exeDir, "pw-update-*.exe"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// verifyAttestationGate verifies build provenance of the downloaded binary.
// Verification fails closed: any mismatch or missing attestation blocks the update unless skipped explicitly.
func verifyAttestationGate(ctx context.Context, client *http.Client, apiBaseURL, repo, filePath, actualSHA string, token string) error {
	if apiBaseURL == defaultGitHubAPI {
		if ghExe, err := exec.LookPath("gh"); err == nil {
			attCtx, attCancel := context.WithTimeout(ctx, metadataTimeout)
			defer attCancel()
			var outBuf bytes.Buffer
			attCmd := exec.CommandContext(attCtx, ghExe, "attestation", "verify", filePath, "--repo", repo)
			attCmd.Stdout = &outBuf
			attCmd.Stderr = &outBuf
			if err := attCmd.Run(); err != nil {
				errOutput := strings.TrimSpace(outBuf.String())
				if errOutput != "" {
					return fmt.Errorf("GitHub artifact attestation verification failed: %w (%s)", err, errOutput)
				}
				return fmt.Errorf("GitHub artifact attestation verification failed: %w", err)
			}
			return nil
		}
	}

	// Fallback when gh CLI is not installed: query GitHub attestations API directly
	url := fmt.Sprintf("%s/repos/%s/attestations/sha256:%s", strings.TrimRight(apiBaseURL, "/"), repo, strings.ToLower(actualSHA))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build attestation query request: %w", err)
	}
	req.Header.Set("User-Agent", "Patchwork-CLI")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query GitHub attestations API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("no GitHub build provenance attestation found for digest %s; refusing to update without authentic provenance (pass --skip-attestation to bypass)", actualSHA)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub attestations API returned status %d", resp.StatusCode)
	}

	var attResp ghAttestationsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestSize)).Decode(&attResp); err != nil {
		return fmt.Errorf("failed to decode attestation response: %w", err)
	}
	if len(attResp.Attestations) == 0 {
		return fmt.Errorf("zero attestations found for digest %s; refusing to update without authentic provenance", actualSHA)
	}
	return nil
}

func performSelfUpdate(ctx context.Context, client *http.Client, apiBaseURL, currentVersion, goarch, exePath string, opts selfUpdateOptions, out io.Writer) error {
	if !opts.force {
		if err := detectManagedInstall(exePath); err != nil {
			return err
		}
	}

	exeDir := filepath.Dir(exePath)
	cleanupStaleUpdateFiles(exeDir, exePath)

	fmt.Fprintf(out, "Checking for Patchwork updates (current: %s)...\n", currentVersion)

	assetName := "pw.exe"
	wingetArch := "x64"
	if goarch == "arm64" {
		assetName = "pw-arm64.exe"
		wingetArch = "arm64"
	}

	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	metaCtx, metaCancel := context.WithTimeout(ctx, metadataTimeout)
	defer metaCancel()

	apiURL := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(apiBaseURL, "/"), githubRepo)
	req, err := http.NewRequestWithContext(metaCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Patchwork-CLI")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestSize)).Decode(&rel); err != nil {
		return fmt.Errorf("failed to decode release info: %w", err)
	}

	if rel.TagName == "" {
		return fmt.Errorf("no release tag found")
	}
	if rel.Draft || rel.Prerelease {
		return fmt.Errorf("release %s is a draft or prerelease; refusing to update", rel.TagName)
	}

	if !isNewerVersion(currentVersion, rel.TagName) {
		fmt.Fprintf(out, "Patchwork is already up to date (%s).\n", currentVersion)
		return nil
	}

	var targetAsset *ghAsset
	var manifestAsset *ghAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == assetName {
			targetAsset = &rel.Assets[i]
		}
		if rel.Assets[i].Name == "KaranBanwasi.Patchwork.installer.yaml" {
			manifestAsset = &rel.Assets[i]
		}
	}
	if targetAsset == nil {
		return fmt.Errorf("release %s does not contain asset %q", rel.TagName, assetName)
	}
	if manifestAsset == nil {
		return fmt.Errorf("release %s has no checksum manifest; refusing to update", rel.TagName)
	}

	// Fetch installer manifest to verify SHA-256 checksum (fail closed)
	manCtx, manCancel := context.WithTimeout(ctx, metadataTimeout)
	defer manCancel()

	reqMan, err := http.NewRequestWithContext(manCtx, http.MethodGet, manifestAsset.BrowserDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build manifest request: %w", err)
	}
	reqMan.Header.Set("User-Agent", "Patchwork-CLI")
	if token != "" && manifestAsset.URL != "" {
		reqMan.URL, _ = reqMan.URL.Parse(manifestAsset.URL)
		reqMan.Header.Set("Authorization", "Bearer "+token)
		reqMan.Header.Set("Accept", "application/octet-stream")
	}

	respMan, err := client.Do(reqMan)
	if err != nil {
		return fmt.Errorf("failed to download checksum manifest: %w", err)
	}
	defer respMan.Body.Close()

	if respMan.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum manifest download returned status %d", respMan.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(respMan.Body, maxManifestSize))
	if err != nil {
		return fmt.Errorf("failed to read checksum manifest: %w", err)
	}

	expectedSHA := extractExpectedSHA256(string(bodyBytes), wingetArch)
	if expectedSHA == "" {
		return fmt.Errorf("could not determine expected SHA256 for %s; refusing to update", assetName)
	}

	fmt.Fprintf(out, "Downloading Patchwork %s (%s)...\n", rel.TagName, assetName)

	dlCtx, dlCancel := context.WithTimeout(ctx, binaryDownloadTime)
	defer dlCancel()

	dlURL := targetAsset.BrowserDownloadURL
	reqDL, err := http.NewRequestWithContext(dlCtx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	reqDL.Header.Set("User-Agent", "Patchwork-CLI")
	if token != "" && targetAsset.URL != "" {
		reqDL.URL, _ = reqDL.URL.Parse(targetAsset.URL)
		reqDL.Header.Set("Authorization", "Bearer "+token)
		reqDL.Header.Set("Accept", "application/octet-stream")
	}

	respDL, err := client.Do(reqDL)
	if err != nil {
		return fmt.Errorf("failed to download release asset: %w", err)
	}
	defer respDL.Body.Close()

	if respDL.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download returned status %d", respDL.StatusCode)
	}

	tmpFile, err := os.CreateTemp(exeDir, "pw-update-*.exe")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
			return fmt.Errorf("cannot write to %s: installed in a protected location; rerun elevated or use the package manager that installed it", exeDir)
		}
		return fmt.Errorf("failed to create temporary file in %s: %w", exeDir, err)
	}
	tmpPath := tmpFile.Name()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), respDL.Body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write downloaded binary: %w", err)
	}
	_ = tmpFile.Close()

	actualSHA := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))
	if !strings.EqualFold(actualSHA, expectedSHA) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("SHA256 checksum mismatch for %s: expected %s, got %s", assetName, expectedSHA, actualSHA)
	}
	fmt.Fprintf(out, "Verified SHA256 checksum (%s).\n", actualSHA[:12])

	// Attestation verification gate: fail closed if attestation fails or is required and missing
	if !opts.skipAttestation {
		if err := verifyAttestationGate(ctx, client, apiBaseURL, githubRepo, tmpPath, actualSHA, token); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("provenance attestation verification gate failed: %w", err)
		}
		fmt.Fprintln(out, "Verified GitHub build provenance attestation.")
	} else {
		fmt.Fprintln(out, "Warning: Skipping build provenance attestation verification (--skip-attestation).")
	}

	_ = os.Chmod(tmpPath, 0755)

	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(tmpPath)
		if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
			return fmt.Errorf("cannot replace %s: installed in a protected location; rerun elevated or use the package manager that installed it", exePath)
		}
		return fmt.Errorf("failed to move running binary: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		// Rollback
		_ = os.Rename(oldPath, exePath)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace binary with new version: %w", err)
	}

	// Best-effort cleanup of .old binary (on Windows this may remain until next update)
	_ = os.Remove(oldPath)

	fmt.Fprintf(out, "Successfully updated Patchwork to %s!\n", rel.TagName)
	return nil
}

func newSelfUpdateCmd() *cobra.Command {
	var opts selfUpdateOptions

	cmd := &cobra.Command{
		Use:     "self-update",
		Aliases: []string{"update"},
		Short:   "Update Patchwork (pw) to the latest release",
		Long:    `Check GitHub Releases for a newer version of Patchwork and update the executable in-place.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to determine current executable path: %w", err)
			}
			exePath, err = filepath.EvalSymlinks(exePath)
			if err != nil {
				return fmt.Errorf("failed to resolve executable symlink: %w", err)
			}

			return performSelfUpdate(
				cmd.Context(),
				http.DefaultClient,
				defaultGitHubAPI,
				version,
				runtime.GOARCH,
				exePath,
				opts,
				cmd.OutOrStdout(),
			)
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force self-update even if installed via Winget or Scoop")
	cmd.Flags().BoolVar(&opts.skipAttestation, "skip-attestation", false, "Skip build provenance attestation verification")
	cmd.Flags().BoolVar(&opts.requireAttestation, "require-attestation", false, "Require build provenance attestation even in offline/custom environments")

	return cmd
}
