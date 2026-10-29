package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const githubRepo = "karan-banwasi/Patchwork"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Draft   bool      `json:"draft"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
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

var selfUpdateCmd = &cobra.Command{
	Use:     "self-update",
	Aliases: []string{"update"},
	Short:   "Update Patchwork (pw) to the latest release",
	Long:    `Check GitHub Releases for a newer version of Patchwork and update the executable in-place.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		defer cancel()

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Checking for Patchwork updates (current: %s)...\n", version)

		assetName := "pw.exe"
		if runtime.GOARCH == "arm64" {
			assetName = "pw-arm64.exe"
		}

		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}

		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Patchwork-CLI")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to query GitHub releases: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var rel ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return fmt.Errorf("failed to decode release info: %w", err)
		}

		if rel.TagName == "" {
			return fmt.Errorf("no release tag found")
		}

		if !isNewerVersion(version, rel.TagName) {
			fmt.Fprintf(out, "Patchwork is already up to date (%s).\n", version)
			return nil
		}

		var targetAsset *ghAsset
		for i := range rel.Assets {
			if rel.Assets[i].Name == assetName {
				targetAsset = &rel.Assets[i]
				break
			}
		}
		if targetAsset == nil {
			return fmt.Errorf("release %s does not contain asset %q", rel.TagName, assetName)
		}

		fmt.Fprintf(out, "Downloading Patchwork %s (%s)...\n", rel.TagName, assetName)

		dlURL := targetAsset.BrowserDownloadURL
		reqDL, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
		if err != nil {
			return err
		}
		reqDL.Header.Set("User-Agent", "Patchwork-CLI")
		if token != "" && targetAsset.URL != "" {
			reqDL.URL, _ = reqDL.URL.Parse(targetAsset.URL)
			reqDL.Header.Set("Authorization", "Bearer "+token)
			reqDL.Header.Set("Accept", "application/octet-stream")
		}

		respDL, err := http.DefaultClient.Do(reqDL)
		if err != nil {
			return fmt.Errorf("failed to download release asset: %w", err)
		}
		defer respDL.Body.Close()

		if respDL.StatusCode != http.StatusOK {
			return fmt.Errorf("asset download returned status %d", respDL.StatusCode)
		}

		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to determine current executable path: %w", err)
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			return fmt.Errorf("failed to resolve executable symlink: %w", err)
		}

		exeDir := filepath.Dir(exePath)
		tmpFile, err := os.CreateTemp(exeDir, "pw-update-*.exe")
		if err != nil {
			return fmt.Errorf("failed to create temporary file in %s: %w", exeDir, err)
		}
		tmpPath := tmpFile.Name()

		if _, err := io.Copy(tmpFile, respDL.Body); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to write downloaded binary: %w", err)
		}
		_ = tmpFile.Close()
		_ = os.Chmod(tmpPath, 0755)

		oldPath := exePath + ".old"
		_ = os.Remove(oldPath)

		if err := os.Rename(exePath, oldPath); err != nil {
			_ = os.Remove(tmpPath)
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
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}
