//go:build windows

package cmd

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstallScript_PowerShell(t *testing.T) {
	psExe, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("powershell.exe not found on Windows; skipping install.ps1 tests")
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}
	testScript := filepath.Join(repoRoot, "tests", "install_test.ps1")

	cmd := exec.Command(psExe, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", testScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install_test.ps1 failed with error %v:\n%s", err, string(out))
	}
	t.Logf("install_test.ps1 output:\n%s", string(out))
}
