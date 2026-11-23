package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMainBuildsAndRuns(t *testing.T) {
	tmpDir := t.TempDir()
	binName := "pw_test_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	outBin := filepath.Join(tmpDir, binName)

	// Build cmd/pw
	buildCmd := exec.Command("go", "build", "-o", outBin, ".")
	buildCmd.Env = os.Environ()
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build cmd/pw: %v\nOutput: %s", err, string(output))
	}

	// Verify executing --help
	helpCmd := exec.Command(outBin, "--help")
	helpOutput, err := helpCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run %s --help: %v\nOutput: %s", outBin, err, string(helpOutput))
	}
	outStr := string(helpOutput)
	if !strings.Contains(outStr, "Patchwork") && !strings.Contains(outStr, "pw") {
		t.Errorf("expected help output to contain 'Patchwork' or 'pw', got: %s", outStr)
	}

	// Verify executing version
	verCmd := exec.Command(outBin, "version")
	verOutput, err := verCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run %s version: %v\nOutput: %s", outBin, err, string(verOutput))
	}
}
