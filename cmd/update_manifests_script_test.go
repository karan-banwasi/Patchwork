package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runUpdateManifestsScript(t *testing.T, bashPath string, args ...string) (string, error) {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to locate repository root: %v", err)
	}
	scriptPath := filepath.ToSlash(filepath.Join(repoRoot, "scripts", "update-manifests.sh"))
	cmdArgs := append([]string{scriptPath}, args...)
	c := exec.Command(bashPath, cmdArgs...)
	out, err := c.CombinedOutput()
	return string(out), err
}

func TestUpdateManifestsScript(t *testing.T) {
	bashPath := findWorkingBash(t)

	const (
		shaX64     = "AaBbCcDdEeFf00112233445566778899AaBbCcDdEeFf00112233445566778899"
		shaArm64   = "11223344556677889900aabbccddeeff11223344556677889900AABBCCDDEEFF"
		tarballSHA = "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	)

	t.Run("rejects missing required arguments", func(t *testing.T) {
		out, err := runUpdateManifestsScript(t, bashPath)
		if err == nil {
			t.Fatalf("expected non-zero exit when required arguments are missing, got output:\n%s", out)
		}
	})

	t.Run("rejects invalid sha256 checksum", func(t *testing.T) {
		out, err := runUpdateManifestsScript(
			t,
			bashPath,
			"--version", "9.8.7",
			"--sha-x64", "not-a-sha",
			"--sha-arm64", shaArm64,
			"--tarball-sha", tarballSHA,
		)
		if err == nil {
			t.Fatalf("expected non-zero exit for invalid sha256 checksum, got output:\n%s", out)
		}
		if !strings.Contains(strings.ToLower(out), "sha") {
			t.Fatalf("expected sha validation error, got:\n%s", out)
		}
	})

	t.Run("updates Formula, Scoop bucket, and Winget manifests cleanly", func(t *testing.T) {
		tmpRoot := t.TempDir()
		relFiles := []string{
			filepath.Join("Formula", "pw.rb"),
			filepath.Join("bucket", "pw.json"),
			filepath.Join("winget", "KaranBanwasi.Patchwork.yaml"),
			filepath.Join("winget", "KaranBanwasi.Patchwork.installer.yaml"),
			filepath.Join("winget", "KaranBanwasi.Patchwork.locale.en-US.yaml"),
		}
		for _, rel := range relFiles {
			src := filepath.Join("..", rel)
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("failed to read %s: %v", src, err)
			}
			dst := filepath.Join(tmpRoot, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				t.Fatalf("failed to create directory for %s: %v", dst, err)
			}
			if err := os.WriteFile(dst, data, 0644); err != nil {
				t.Fatalf("failed to write %s: %v", dst, err)
			}
		}

		out, err := runUpdateManifestsScript(
			t,
			bashPath,
			"--version", "9.8.7",
			"--tag", "v9.8.7",
			"--sha-x64", shaX64,
			"--sha-arm64", shaArm64,
			"--tarball-sha", tarballSHA,
			"--repo", "karan-banwasi/Patchwork",
			"--root", filepath.ToSlash(tmpRoot),
		)
		if err != nil {
			t.Fatalf("expected update-manifests.sh to succeed, got %v:\n%s", err, out)
		}

		formulaBytes, err := os.ReadFile(filepath.Join(tmpRoot, "Formula", "pw.rb"))
		if err != nil {
			t.Fatalf("failed to read updated Formula/pw.rb: %v", err)
		}
		formulaText := string(formulaBytes)
		if !strings.Contains(formulaText, "https://github.com/karan-banwasi/Patchwork/archive/refs/tags/v9.8.7.tar.gz") {
			t.Fatalf("Formula/pw.rb missing expected tarball URL, got:\n%s", formulaText)
		}
		if !strings.Contains(formulaText, strings.ToLower(tarballSHA)) {
			t.Fatalf("Formula/pw.rb missing lowercase tarball sha256, got:\n%s", formulaText)
		}

		scoopBytes, err := os.ReadFile(filepath.Join(tmpRoot, "bucket", "pw.json"))
		if err != nil {
			t.Fatalf("failed to read updated bucket/pw.json: %v", err)
		}
		type scoopArch struct {
			URL  string `json:"url"`
			Hash string `json:"hash"`
		}
		type scoopManifest struct {
			Version      string               `json:"version"`
			Architecture map[string]scoopArch `json:"architecture"`
		}
		var scoop scoopManifest
		if err := json.Unmarshal(scoopBytes, &scoop); err != nil {
			t.Fatalf("failed to unmarshal bucket/pw.json: %v\n%s", err, string(scoopBytes))
		}
		if scoop.Version != "9.8.7" {
			t.Fatalf("expected Scoop version 9.8.7, got %q", scoop.Version)
		}
		if got := scoop.Architecture["64bit"]; got.URL != "https://github.com/karan-banwasi/Patchwork/releases/download/v9.8.7/pw.exe" || got.Hash != strings.ToLower(shaX64) {
			t.Fatalf("unexpected Scoop 64bit architecture entry: %+v", got)
		}
		if got := scoop.Architecture["arm64"]; got.URL != "https://github.com/karan-banwasi/Patchwork/releases/download/v9.8.7/pw-arm64.exe" || got.Hash != strings.ToLower(shaArm64) {
			t.Fatalf("unexpected Scoop arm64 architecture entry: %+v", got)
		}

		wingetFiles := []string{
			"KaranBanwasi.Patchwork.yaml",
			"KaranBanwasi.Patchwork.locale.en-US.yaml",
			"KaranBanwasi.Patchwork.installer.yaml",
		}
		wingetContents := make(map[string]string, len(wingetFiles))
		for _, name := range wingetFiles {
			b, err := os.ReadFile(filepath.Join(tmpRoot, "winget", name))
			if err != nil {
				t.Fatalf("failed to read updated winget/%s: %v", name, err)
			}
			if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
				t.Fatalf("winget/%s unexpectedly has a UTF-8 BOM", name)
			}
			if bytes.Contains(b, []byte("\r\n")) {
				t.Fatalf("winget/%s unexpectedly contains CRLF line endings", name)
			}
			text := string(b)
			if !strings.Contains(text, "PackageVersion: 9.8.7") {
				t.Fatalf("winget/%s missing PackageVersion: 9.8.7, got:\n%s", name, text)
			}
			wingetContents[name] = text
		}

		localeText := wingetContents["KaranBanwasi.Patchwork.locale.en-US.yaml"]
		if !strings.Contains(localeText, "ReleaseNotesUrl: https://github.com/karan-banwasi/Patchwork/releases/tag/v9.8.7") {
			t.Fatalf("locale manifest missing updated ReleaseNotesUrl, got:\n%s", localeText)
		}

		installerText := wingetContents["KaranBanwasi.Patchwork.installer.yaml"]
		wantInstallerFragments := []string{
			"InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v9.8.7/pw.exe",
			"InstallerSha256: " + strings.ToUpper(shaX64),
			"InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v9.8.7/pw-arm64.exe",
			"InstallerSha256: " + strings.ToUpper(shaArm64),
		}
		for _, want := range wantInstallerFragments {
			if !strings.Contains(installerText, want) {
				t.Fatalf("installer manifest missing %q, got:\n%s", want, installerText)
			}
		}
	})
}
