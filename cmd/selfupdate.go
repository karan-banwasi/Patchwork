package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/httputil"
	ver "github.com/karan-banwasi/patchwork/internal/version"
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
	offline            bool
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

type sigstoreBundle struct {
	MediaType            string                 `json:"mediaType"`
	VerificationMaterial *bundleVerificationMat `json:"verificationMaterial"`
	DSSEEnvelope         *dsseEnvelope          `json:"dsseEnvelope"`
}

type bundleVerificationMat struct {
	Certificate          *bundleCert      `json:"certificate"`
	X509CertificateChain *bundleCertChain `json:"x509CertificateChain"`
}

type bundleCert struct {
	RawBytes string `json:"rawBytes"`
}

type bundleCertChain struct {
	Certificates []bundleCert `json:"certificates"`
}

type dsseEnvelope struct {
	Payload     string          `json:"payload"`
	PayloadType string          `json:"payloadType"`
	Signatures  []dsseSignature `json:"signatures"`
}

type dsseSignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

type inTotoStatement struct {
	Type          string          `json:"_type"`
	Subject       []inTotoSubject `json:"subject"`
	PredicateType string          `json:"predicateType"`
}

type inTotoSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// isNewerVersion compares two semantic version tags (e.g. "v1.0.5" vs "v1.0.4").
func isNewerVersion(current, latest string) bool {
	return ver.IsUpgrade("selfupdate", current, latest)
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
	case strings.Contains(strings.ToLower(exePath), "/cellar/patchwork/") || strings.Contains(strings.ToLower(exePath), "/opt/homebrew/") || strings.Contains(strings.ToLower(exePath), "/home/linuxbrew/") || strings.Contains(strings.ToLower(exePath), "/.linuxbrew/"):
		return fmt.Errorf("Patchwork was installed with Homebrew; run: brew upgrade patchwork (or pass --force to override)")
	}
	return nil
}

// resolveBinaryAsset maps GOOS and GOARCH to the expected release binary asset name.
func resolveBinaryAsset(goos, goarch string) (string, error) {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return "pw.exe", nil
		case "arm64":
			return "pw-arm64.exe", nil
		default:
			return "", fmt.Errorf("unsupported architecture for windows: %s", goarch)
		}
	case "darwin":
		switch goarch {
		case "arm64":
			return "pw-darwin-arm64", nil
		case "amd64":
			return "pw-darwin-amd64", nil
		default:
			return "", fmt.Errorf("unsupported architecture for darwin: %s", goarch)
		}
	case "linux":
		switch goarch {
		case "amd64":
			return "pw-linux-amd64", nil
		case "arm64":
			return "pw-linux-arm64", nil
		default:
			return "", fmt.Errorf("unsupported architecture for linux: %s", goarch)
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}
}

// parseSHA256Sums parses standard SHA256SUMS content (lines of "<hash> [*]<filename>")
// and returns a map of filename to uppercase hex SHA256.
func parseSHA256Sums(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := fields[0]
		if len(hash) != 64 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		rest := strings.TrimSpace(line[len(hash):])
		rest = strings.TrimPrefix(rest, "*")
		rest = strings.TrimSpace(rest)
		rest = strings.TrimPrefix(rest, "./")
		rest = strings.TrimPrefix(rest, `.\`)
		rest = strings.ReplaceAll(rest, `\`, "/")
		var filename string
		if idx := strings.LastIndex(rest, "/"); idx != -1 {
			filename = rest[idx+1:]
		} else {
			filename = rest
		}
		if filename != "" {
			result[filename] = strings.ToUpper(hash)
		}
	}
	return result
}

// downloadAssetBytes downloads small metadata or manifest assets (limited to maxManifestSize).
func downloadAssetBytes(ctx context.Context, client *http.Client, asset *ghAsset, token string) ([]byte, error) {
	manCtx, manCancel := context.WithTimeout(ctx, metadataTimeout)
	defer manCancel()

	req, err := http.NewRequestWithContext(manCtx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build asset request: %w", err)
	}
	req.Header.Set("User-Agent", "Patchwork-CLI")
	if token != "" && asset.URL != "" {
		req.URL, _ = req.URL.Parse(asset.URL)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return bodyBytes, nil
}

// cleanupStaleUpdateFiles removes leftover temporary binaries from interrupted self-updates.
func cleanupStaleUpdateFiles(exeDir, exePath string) {
	_ = os.Remove(exePath + ".old")
	matches, err := filepath.Glob(filepath.Join(exeDir, "pw-update-*"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

func verifyAttestationBundle(bundleRaw []byte, repo, pinnedWorkflow, actualSHA string) error {
	var bundle sigstoreBundle
	if err := json.Unmarshal(bundleRaw, &bundle); err != nil {
		return fmt.Errorf("failed to parse sigstore bundle: %w", err)
	}

	var certBytes []byte
	if bundle.VerificationMaterial != nil {
		if bundle.VerificationMaterial.Certificate != nil && bundle.VerificationMaterial.Certificate.RawBytes != "" {
			b, err := base64.StdEncoding.DecodeString(bundle.VerificationMaterial.Certificate.RawBytes)
			if err == nil {
				certBytes = b
			}
		} else if bundle.VerificationMaterial.X509CertificateChain != nil && len(bundle.VerificationMaterial.X509CertificateChain.Certificates) > 0 {
			b, err := base64.StdEncoding.DecodeString(bundle.VerificationMaterial.X509CertificateChain.Certificates[0].RawBytes)
			if err == nil {
				certBytes = b
			}
		}
	}
	if len(certBytes) == 0 {
		return errors.New("missing certificate in attestation verification material")
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify signer workflow identity strictly in certificate Subject Alternative Name (SAN) URIs.
	// In Sigstore Fulcio certificates for GitHub Actions provenance, the SAN URI format is:
	// https://github.com/<owner>/<repo>/<workflow_path>@<ref>
	// We strictly anchor the scheme, host, and workflow path before any '@' ref delimiter.
	cleanedPinnedWorkflow := strings.Trim(pinnedWorkflow, "/")
	expectedURI := "https://github.com/" + cleanedPinnedWorkflow
	expectedPath := "/" + cleanedPinnedWorkflow

	workflowMatched := false
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") {
			continue
		}
		if u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
			continue
		}

		raw := u.String()
		if idx := strings.Index(raw, "@"); idx != -1 {
			if idx == len(raw)-1 {
				continue
			}
			raw = raw[:idx]
		}

		path := u.Path
		if idx := strings.Index(path, "@"); idx != -1 {
			path = path[:idx]
		}

		if raw == expectedURI && path == expectedPath {
			workflowMatched = true
			break
		}
	}
	if !workflowMatched {
		return fmt.Errorf("signing certificate does not match expected signer workflow %q", pinnedWorkflow)
	}

	pubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("unsupported public key type in certificate: %T", cert.PublicKey)
	}

	if bundle.DSSEEnvelope == nil || len(bundle.DSSEEnvelope.Signatures) == 0 {
		return errors.New("missing DSSE envelope or signatures in bundle")
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(bundle.DSSEEnvelope.Payload)
	if err != nil {
		return fmt.Errorf("failed to decode DSSE payload: %w", err)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(bundle.DSSEEnvelope.Signatures[0].Sig)
	if err != nil {
		return fmt.Errorf("failed to decode DSSE signature: %w", err)
	}

	pae := []byte(fmt.Sprintf("DSSEv1 %d %s %d ", len(bundle.DSSEEnvelope.PayloadType), bundle.DSSEEnvelope.PayloadType, len(payloadBytes)))
	pae = append(pae, payloadBytes...)
	paeHash := sha256.Sum256(pae)

	verifiedSig := ecdsa.VerifyASN1(pubKey, paeHash[:], sigBytes)
	if !verifiedSig && len(sigBytes) == 64 {
		r := new(big.Int).SetBytes(sigBytes[:32])
		s := new(big.Int).SetBytes(sigBytes[32:])
		verifiedSig = ecdsa.Verify(pubKey, paeHash[:], r, s)
	}
	if !verifiedSig {
		return errors.New("cryptographic signature verification failed")
	}

	var stmt inTotoStatement
	if err := json.Unmarshal(payloadBytes, &stmt); err != nil {
		return fmt.Errorf("failed to parse in-toto statement: %w", err)
	}

	digestMatched := false
	for _, subj := range stmt.Subject {
		if strings.EqualFold(subj.Digest["sha256"], actualSHA) {
			digestMatched = true
			break
		}
	}
	if !digestMatched {
		return fmt.Errorf("attestation statement subject digests do not match binary sha256 %s", actualSHA)
	}

	return nil
}

// verifyAttestationGate verifies build provenance of the downloaded binary.
// Verification fails closed: any mismatch or missing attestation blocks the update unless skipped explicitly.
func verifyAttestationGate(ctx context.Context, client *http.Client, apiBaseURL, repo, filePath, actualSHA string, token string) error {
	pinnedWorkflow := fmt.Sprintf("%s/.github/workflows/release.yml", repo)

	if apiBaseURL == defaultGitHubAPI {
		if ghExe, err := exec.LookPath("gh"); err == nil {
			attCtx, attCancel := context.WithTimeout(ctx, metadataTimeout)
			defer attCancel()
			var outBuf bytes.Buffer
			attCmd := exec.CommandContext(attCtx, ghExe, "attestation", "verify", filePath, "--repo", repo, "--signer-workflow", pinnedWorkflow)
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

	var lastErr error
	verified := false
	for i, att := range attResp.Attestations {
		if len(att.Bundle) == 0 {
			lastErr = fmt.Errorf("attestation bundle [%d] is empty", i)
			continue
		}
		if err := verifyAttestationBundle(att.Bundle, repo, pinnedWorkflow, actualSHA); err != nil {
			lastErr = err
			continue
		}
		verified = true
		break
	}

	if !verified {
		if lastErr != nil {
			return fmt.Errorf("cryptographic verification of build provenance attestation failed: %w", lastErr)
		}
		return fmt.Errorf("zero valid cryptographic attestations found for digest %s", actualSHA)
	}

	return nil
}

func performSelfUpdate(ctx context.Context, client *http.Client, apiBaseURL, currentVersion, goos, goarch, exePath string, opts selfUpdateOptions, out io.Writer) error {
	if opts.skipAttestation && opts.requireAttestation {
		return fmt.Errorf("cannot specify both --skip-attestation and --require-attestation")
	}

	if !opts.force {
		if err := detectManagedInstall(exePath); err != nil {
			return err
		}
	}

	exeDir := filepath.Dir(exePath)
	cleanupStaleUpdateFiles(exeDir, exePath)

	fmt.Fprintf(out, "Checking for Patchwork updates (current: %s)...\n", currentVersion)

	assetName, err := resolveBinaryAsset(goos, goarch)
	if err != nil {
		return err
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
	var sha256SumsAsset *ghAsset
	var wingetYamlAsset *ghAsset

	for i := range rel.Assets {
		switch rel.Assets[i].Name {
		case assetName:
			targetAsset = &rel.Assets[i]
		case "SHA256SUMS":
			sha256SumsAsset = &rel.Assets[i]
		case "KaranBanwasi.Patchwork.installer.yaml":
			wingetYamlAsset = &rel.Assets[i]
		}
	}

	// Compatibility fallback for Windows asset names
	if targetAsset == nil && goos == "windows" {
		altName := ""
		if goarch == "amd64" {
			altName = "pw-windows-amd64.exe"
		} else if goarch == "arm64" {
			altName = "pw-windows-arm64.exe"
		}
		if altName != "" {
			for i := range rel.Assets {
				if rel.Assets[i].Name == altName {
					targetAsset = &rel.Assets[i]
					assetName = altName
					break
				}
			}
		}
	}

	if targetAsset == nil {
		return fmt.Errorf("release %s does not contain asset %q", rel.TagName, assetName)
	}

	var expectedSHA string
	if sha256SumsAsset != nil {
		bodyBytes, err := downloadAssetBytes(ctx, client, sha256SumsAsset, token)
		if err != nil {
			return fmt.Errorf("failed to download checksum manifest (SHA256SUMS): %w", err)
		}
		sums := parseSHA256Sums(string(bodyBytes))
		expectedSHA = sums[assetName]
		if expectedSHA == "" {
			return fmt.Errorf("release %s SHA256SUMS does not contain checksum for %s; refusing to update", rel.TagName, assetName)
		}
	} else if goos == "windows" && wingetYamlAsset != nil {
		bodyBytes, err := downloadAssetBytes(ctx, client, wingetYamlAsset, token)
		if err != nil {
			return fmt.Errorf("failed to download checksum manifest: %w", err)
		}
		wingetArch := "x64"
		if goarch == "arm64" {
			wingetArch = "arm64"
		}
		expectedSHA = extractExpectedSHA256(string(bodyBytes), wingetArch)
		if expectedSHA == "" {
			return fmt.Errorf("could not determine expected SHA256 for %s; refusing to update", assetName)
		}
	} else {
		if goos == "windows" {
			return fmt.Errorf("release %s has no checksum manifest (neither SHA256SUMS nor winget installer yaml); refusing to update", rel.TagName)
		}
		return fmt.Errorf("release %s has no SHA256SUMS checksum manifest; refusing to update", rel.TagName)
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

	tempPattern := "pw-update-*"
	if goos == "windows" {
		tempPattern = "pw-update-*.exe"
	}

	tmpFile, err := os.CreateTemp(exeDir, tempPattern)
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
			if opts.offline {
				return errors.New("self-update requires network access and cannot run in --offline mode")
			}

			exePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to determine current executable path: %w", err)
			}
			exePath, err = filepath.EvalSymlinks(exePath)
			if err != nil {
				return fmt.Errorf("failed to resolve executable symlink: %w", err)
			}

			client := httputil.NewClient(httputil.WithTimeout(metadataTimeout))
			return performSelfUpdate(
				cmd.Context(),
				client,
				defaultGitHubAPI,
				version,
				runtime.GOOS,
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
	cmd.Flags().BoolVar(&opts.offline, "offline", false, "Disable network access")

	return cmd
}
