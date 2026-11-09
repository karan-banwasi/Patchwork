package cmd

import (
	"context"
	"os"
	"path/filepath"
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
	// Test Windows manager aliases
	winReg := getDefaultRegistryForOS("windows")
	for _, name := range []string{"Winget", "WINGET", "scoop", "SCOOP", "choco", "CHOCO", "chocolatey", "Chocolatey", "npm", "dotnet", "dotnet-tool", "uv", "uv-tool", "cargo", "rust", "psresource", "powershell", "psresourceget"} {
		mgr, exists := winReg.GetManager(name)
		if !exists {
			t.Errorf("expected windows manager %q to be recognized", name)
		}
		if mgr == nil {
			t.Errorf("expected non-nil manager for %q", name)
		}
	}

	// Test Darwin and Linux manager aliases (including brew)
	for _, osName := range []string{"darwin", "linux"} {
		unixReg := getDefaultRegistryForOS(osName)
		for _, name := range []string{"brew", "BREW", "homebrew", "Homebrew", "npm", "dotnet", "dotnet-tool", "uv", "uv-tool", "cargo", "rust"} {
			mgr, exists := unixReg.GetManager(name)
			if !exists {
				t.Errorf("expected %s manager %q to be recognized", osName, name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	}

	// Test current host OS registry
	currentReg := getDefaultRegistry()
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
