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
	expectedManagers := []string{"Configuration:", "Package Managers:", "npm", "dotnet", "uv", "cargo"}
	if runtime.GOOS == "windows" {
		expectedManagers = append(expectedManagers, "winget", "scoop", "choco", "psresource")
	} else {
		expectedManagers = append(expectedManagers, "brew")
	}
	for _, expected := range expectedManagers {
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
		t.Errorf("expected warning about multiple pw installations, got: %s", out)
	}
}

func TestGetDefaultRegistry(t *testing.T) {
	reg := getDefaultRegistry()
	names := reg.ManagerNames()
	for _, expected := range []string{"npm", "dotnet", "uv", "cargo"} {
		if _, ok := reg.GetManager(expected); !ok {
			t.Errorf("expected registry to contain %s, got: %v", expected, names)
		}
	}

	if runtime.GOOS == "windows" {
		for _, expected := range []string{"winget", "scoop", "choco", "psresource"} {
			if _, ok := reg.GetManager(expected); !ok {
				t.Errorf("expected windows registry to contain %s, got: %v", expected, names)
			}
		}
		if _, ok := reg.GetManager("brew"); ok {
			t.Errorf("expected windows registry NOT to contain brew")
		}
	} else {
		if _, ok := reg.GetManager("brew"); !ok {
			t.Errorf("expected unix registry to contain brew, got: %v", names)
		}
		for _, excluded := range []string{"winget", "scoop", "choco", "psresource"} {
			if _, ok := reg.GetManager(excluded); ok {
				t.Errorf("expected unix registry NOT to contain %s", excluded)
			}
		}
	}
}
