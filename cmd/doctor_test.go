package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctorCmd(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
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
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"cleanup", "-m", "nonexistent-mgr"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown package manager") {
		t.Fatalf("expected unknown package manager error, got: %v", err)
	}
}

func TestFindPWInstallationsOnPath_Duplicates(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	binName := "pw.exe"
	if runtime.GOOS != "windows" {
		binName = "pw"
	}
	_ = os.WriteFile(filepath.Join(dir1, binName), []byte("1"), 0755)
	_ = os.WriteFile(filepath.Join(dir2, binName), []byte("2"), 0755)

	pathEnv := dir1 + string(os.PathListSeparator) + dir2
	found := findPWInstallationsOnPath(pathEnv)
	if len(found) != 2 {
		t.Fatalf("expected 2 pw installations on PATH, got %d: %v", len(found), found)
	}
}

func TestDoctorCmd_WarnsOnDuplicatePWOnPath(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	binName := "pw.exe"
	if runtime.GOOS != "windows" {
		binName = "pw"
	}
	if err := os.WriteFile(filepath.Join(dir1, binName), []byte("1"), 0755); err != nil {
		t.Fatalf("failed to create dummy pw in dir1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, binName), []byte("2"), 0755); err != nil {
		t.Fatalf("failed to create dummy pw in dir2: %v", err)
	}

	pathEnv := dir1 + string(os.PathListSeparator) + dir2
	t.Setenv("PATH", pathEnv)
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error on doctor: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Warning: Multiple pw installations found on PATH") {
		t.Errorf("expected warning about multiple pw installations, got:\n%s", out)
	}
}
