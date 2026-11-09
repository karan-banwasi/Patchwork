package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestCheckCmd_UnknownManager(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"check", "-m", "unknown-mgr"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown manager, got nil")
	}
	if !strings.Contains(err.Error(), "unknown package manager") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCheckCmd_ValidManagerAliases(t *testing.T) {
	currentReg := getDefaultRegistry()
	if runtime.GOOS == "windows" {
		for _, name := range []string{"Winget", "WINGET", "scoop", "SCOOP", "choco", "CHOCO", "chocolatey", "Chocolatey", "npm", "dotnet", "dotnet-tool", "uv", "uv-tool", "cargo", "rust", "psresource", "powershell", "psresourceget"} {
			mgr, exists := currentReg.GetManager(name)
			if !exists {
				t.Errorf("expected windows manager %q to be recognized", name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	} else {
		for _, name := range []string{"brew", "BREW", "homebrew", "Homebrew", "npm", "dotnet", "dotnet-tool", "uv", "uv-tool", "cargo", "rust"} {
			mgr, exists := currentReg.GetManager(name)
			if !exists {
				t.Errorf("expected unix manager %q to be recognized", name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	}

	for _, name := range currentReg.ManagerNames() {
		mgr, exists := currentReg.GetManager(name)
		if !exists || mgr == nil {
			t.Errorf("expected manager %q in current registry to be recognized", name)
		}
	}
}

type mockCheckManager struct {
	name    string
	updates []pm.PackageUpdate
}

func (m *mockCheckManager) Name() string                         { return m.name }
func (m *mockCheckManager) Aliases() []string                    { return nil }
func (m *mockCheckManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockCheckManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return m.updates, nil
}
func (m *mockCheckManager) UpgradePackage(ctx context.Context, id string) error { return nil }
func (m *mockCheckManager) ValidateID(id string) error                          { return nil }

func TestCheckCmd_CorruptConfigFailsClosed(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	if err := os.WriteFile(tmpCfg, []byte("{ corrupt: json"), 0600); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	mockMgr := &mockCheckManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "test.pkg", Name: "TestPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"check", "--no-refresh"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected pw check to fail on corrupt config, got nil")
	}
	if !strings.Contains(err.Error(), "cannot verify pinned packages") {
		t.Errorf("expected error about verifying pinned packages, got: %v", err)
	}

	// With --include-pinned, config error is bypassed
	cmdInclude := NewRootCmd()
	cmdInclude.SetArgs([]string{"check", "--no-refresh", "--include-pinned"})
	errInclude := cmdInclude.Execute()
	if errInclude != nil {
		t.Errorf("expected --include-pinned to bypass config load error, got: %v", errInclude)
	}
}

type mockWarningManager struct {
	mockCheckManager
	refreshErr error
	warnings   []error
}

func (m *mockWarningManager) Refresh(ctx context.Context) error {
	return m.refreshErr
}

func (m *mockWarningManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	return m.updates, m.warnings, nil
}

func TestCheckCmd_SurfacesWarningsToStderr(t *testing.T) {
	mockMgr := &mockWarningManager{
		mockCheckManager: mockCheckManager{
			name: "mockpm",
			updates: []pm.PackageUpdate{
				{Id: "test.pkg", Name: "TestPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
			},
		},
		refreshErr: fmt.Errorf("network connection refused"),
		warnings:   []error{fmt.Errorf("partial registry failure for 1 package")},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected check to succeed despite warnings, got: %v", err)
	}

	errOutput := stderr.String()
	if !strings.Contains(errOutput, "Warning: [mockpm] refresh failed: network connection refused") {
		t.Errorf("expected refresh warning in stderr, got: %s", errOutput)
	}
	if !strings.Contains(errOutput, "Warning: partial registry failure for 1 package") {
		t.Errorf("expected partial fetch warning in stderr, got: %s", errOutput)
	}

	stdOutput := stdout.String()
	if !strings.Contains(stdOutput, "test.pkg") {
		t.Errorf("expected updates table in stdout, got: %s", stdOutput)
	}
}

func TestCheckCmd_DefaultTimeoutIsZero(t *testing.T) {
	cmd := newCheckCmd()
	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatalf("expected --timeout flag to exist")
	}
	if f.DefValue != "0s" && f.DefValue != "0" {
		t.Errorf("expected default timeout to be 0s, got: %s", f.DefValue)
	}
}
