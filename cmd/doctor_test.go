package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCmd(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"doctor"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error on doctor: %v", err)
	}
	out := buf.String()
	for _, expected := range []string{"Configuration:", "Package Managers:", "winget", "scoop", "choco", "npm", "dotnet", "uv", "cargo", "psresource"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected doctor output to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestCleanupCmd_UnknownManager(t *testing.T) {
	cleanupManager = ""
	defer func() { cleanupManager = "" }()

	rootCmd.SetArgs([]string{"cleanup", "-m", "nonexistent-mgr"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown package manager") {
		t.Fatalf("expected unknown package manager error, got: %v", err)
	}
}
