package cmd

import (
	"strings"
	"testing"
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
