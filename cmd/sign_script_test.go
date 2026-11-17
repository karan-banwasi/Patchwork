package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func findWorkingBash(t *testing.T) string {
	t.Helper()
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
		)
	}
	if p, err := exec.LookPath("bash"); err == nil {
		candidates = append(candidates, p)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if out, err := exec.Command(c, "-c", "exit 0").CombinedOutput(); err == nil {
			_ = out
			return c
		}
	}
	t.Skip("no working bash executable available; skipping sign-windows.sh tests")
	return ""
}

func runSignScript(t *testing.T, bashPath string, extraEnv map[string]string, args ...string) (string, error) {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to locate repository root: %v", err)
	}
	scriptPath := filepath.ToSlash(filepath.Join(repoRoot, "scripts", "sign-windows.sh"))
	cmdArgs := append([]string{scriptPath}, args...)
	c := exec.Command(bashPath, cmdArgs...)

	baseEnv := make([]string, 0, len(os.Environ())+len(extraEnv))
	azureKeys := map[string]bool{
		"AZURE_TENANT_ID":                true,
		"AZURE_CLIENT_ID":                true,
		"AZURE_CLIENT_SECRET":            true,
		"AZURE_ACCESS_TOKEN":             true,
		"AZURE_TRUSTED_SIGNING_ENDPOINT": true,
		"AZURE_TRUSTED_SIGNING_ACCOUNT":  true,
		"AZURE_TRUSTED_SIGNING_PROFILE":  true,
		"REQUIRE_WINDOWS_SIGNING":        true,
		"JSIGN_CMD":                      true,
		"ACTIONS_ID_TOKEN_REQUEST_URL":   true,
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": true,
	}
	for _, entry := range os.Environ() {
		k, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if azureKeys[strings.ToUpper(k)] {
			continue
		}
		baseEnv = append(baseEnv, entry)
	}
	for k, v := range extraEnv {
		baseEnv = append(baseEnv, k+"="+v)
	}
	c.Env = baseEnv

	out, err := c.CombinedOutput()
	return string(out), err
}

func TestSignWindowsScript(t *testing.T) {
	bashPath := findWorkingBash(t)
	tmpDir := t.TempDir()
	dummyExe := filepath.Join(tmpDir, "pw.exe")
	if err := os.WriteFile(dummyExe, []byte("MZ"), 0644); err != nil {
		t.Fatalf("failed to create dummy binary: %v", err)
	}
	dummyExeArg := filepath.ToSlash(dummyExe)

	t.Run("skips cleanly when unconfigured", func(t *testing.T) {
		out, err := runSignScript(t, bashPath, map[string]string{}, dummyExeArg)
		if err != nil {
			t.Fatalf("expected exit 0 when unconfigured, got %v:\n%s", err, out)
		}
		if !strings.Contains(out, "skipping Windows Authenticode signing") {
			t.Fatalf("expected skip notice, got:\n%s", out)
		}
	})

	t.Run("fails closed when REQUIRE_WINDOWS_SIGNING is true", func(t *testing.T) {
		out, err := runSignScript(t, bashPath, map[string]string{
			"REQUIRE_WINDOWS_SIGNING": "true",
		}, dummyExeArg)
		if err == nil {
			t.Fatalf("expected non-zero exit when REQUIRE_WINDOWS_SIGNING=true, got output:\n%s", out)
		}
		if !strings.Contains(out, "AZURE_CLIENT_ID is not set") {
			t.Fatalf("expected missing AZURE_CLIENT_ID error, got:\n%s", out)
		}
	})

	t.Run("rejects incomplete configuration", func(t *testing.T) {
		out, err := runSignScript(t, bashPath, map[string]string{
			"AZURE_CLIENT_ID": "client-123",
		}, dummyExeArg)
		if err == nil {
			t.Fatalf("expected non-zero exit for partial config, got output:\n%s", out)
		}
		if !strings.Contains(out, "AZURE_TRUSTED_SIGNING_ACCOUNT") || !strings.Contains(out, "AZURE_TRUSTED_SIGNING_PROFILE") {
			t.Fatalf("expected missing account/profile error details, got:\n%s", out)
		}
	})

	t.Run("invokes jsign with normalized endpoint and expected flags", func(t *testing.T) {
		logFile := filepath.Join(tmpDir, "jsign-args.txt")
		mockJsign := filepath.Join(tmpDir, "mock-jsign.sh")
		mockBody := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > " + "'" + filepath.ToSlash(logFile) + "'\n"
		if err := os.WriteFile(mockJsign, []byte(mockBody), 0755); err != nil {
			t.Fatalf("failed to write mock jsign script: %v", err)
		}

		out, err := runSignScript(t, bashPath, map[string]string{
			"AZURE_ACCESS_TOKEN":             "mock-token",
			"AZURE_TRUSTED_SIGNING_ENDPOINT": "https://eus.codesigning.azure.net/",
			"AZURE_TRUSTED_SIGNING_ACCOUNT":  "my-acct",
			"AZURE_TRUSTED_SIGNING_PROFILE":  "my-prof",
			"JSIGN_CMD":                      filepath.ToSlash(mockJsign),
		}, dummyExeArg)
		if err != nil {
			t.Fatalf("expected mock signing to succeed, got %v:\n%s", err, out)
		}

		argsBytes, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("failed to read logged jsign arguments: %v", err)
		}
		gotArgs := strings.Join(strings.Fields(string(argsBytes)), " ")
		wantFragments := []string{
			"--storetype TRUSTEDSIGNING",
			"--keystore eus.codesigning.azure.net",
			"--storepass mock-token",
			"--alias my-acct/my-prof",
			"--alg SHA-256",
			"--tsaurl http://timestamp.acs.microsoft.com",
			"--tsmode RFC3161",
			dummyExeArg,
		}
		for _, want := range wantFragments {
			if !strings.Contains(gotArgs, want) {
				t.Fatalf("expected jsign args to contain %q, got: %s", want, gotArgs)
			}
		}
	})
}
