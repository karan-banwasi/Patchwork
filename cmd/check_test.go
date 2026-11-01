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
	reg := getDefaultRegistry()
	for _, name := range []string{"Winget", "WINGET", "scoop", "SCOOP", "choco", "CHOCO", "chocolatey", "Chocolatey", "npm", "dotnet", "dotnet-tool", "uv", "uv-tool", "cargo", "rust", "psresource", "powershell", "psresourceget"} {
		mgr, exists := reg.GetManager(name)
		if !exists {
			t.Errorf("expected manager %q to be recognized", name)
		}
		if mgr == nil {
			t.Errorf("expected non-nil manager for %q", name)
		}
	}
}
