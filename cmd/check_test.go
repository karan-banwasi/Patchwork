package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/config"
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
	crossPlatformAliases := []string{
		"npm", "pnpm", "pnpm-global", "bun", "bun-global",
		"dotnet", "dotnet-tool", "uv", "uv-tool", "pipx", "pipx-app",
		"cargo", "rust", "rustup", "rust-toolchain", "gem", "rubygems", "ruby",
		"mise", "rtx", "asdf", "vscode", "code", "vs-code",
	}
	for _, name := range crossPlatformAliases {
		mgr, exists := currentReg.GetManager(name)
		if !exists {
			t.Errorf("expected cross-platform manager %q to be recognized", name)
		}
		if mgr == nil {
			t.Errorf("expected non-nil manager for %q", name)
		}
	}

	switch runtime.GOOS {
	case "windows":
		for _, name := range []string{"Winget", "WINGET", "scoop", "SCOOP", "choco", "CHOCO", "chocolatey", "Chocolatey", "psresource", "powershell", "psresourceget"} {
			mgr, exists := currentReg.GetManager(name)
			if !exists {
				t.Errorf("expected windows manager %q to be recognized", name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	case "darwin":
		for _, name := range []string{"brew", "BREW", "homebrew", "Homebrew", "mas", "macappstore", "appstore"} {
			mgr, exists := currentReg.GetManager(name)
			if !exists {
				t.Errorf("expected darwin manager %q to be recognized", name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	case "linux":
		for _, name := range []string{
			"brew", "BREW", "homebrew", "Homebrew",
			"apt", "apt-get", "debian", "ubuntu",
			"dnf", "yum", "fedora", "rhel",
			"pacman", "arch",
			"zypper", "opensuse", "suse",
			"flatpak", "flathub",
			"snap", "snapcraft", "snappy",
		} {
			mgr, exists := currentReg.GetManager(name)
			if !exists {
				t.Errorf("expected linux manager %q to be recognized", name)
			}
			if mgr == nil {
				t.Errorf("expected non-nil manager for %q", name)
			}
		}
	default:
		for _, name := range []string{"brew", "BREW", "homebrew", "Homebrew"} {
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

func TestCheckCmd_OfflineFlag(t *testing.T) {
	cmd := newCheckCmd()
	f := cmd.Flags().Lookup("offline")
	if f == nil {
		t.Fatalf("expected --offline flag to exist")
	}
	if f.DefValue != "false" {
		t.Errorf("expected default offline to be false, got: %s", f.DefValue)
	}
}

func TestCheckCmd_JsonSchemaVersion(t *testing.T) {
	mockMgr := &mockCheckManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "test.pkg", Name: "TestPkg", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	cmd := newCheckCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected check --json to succeed, got: %v", err)
	}

	var parsed struct {
		SchemaVersion string `json:"schema_version"`
		Count         int    `json:"count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to decode json output: %v (output: %s)", err, stdout.String())
	}

	if parsed.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version '1.0.0', got %q", parsed.SchemaVersion)
	}
	if parsed.Count != 1 {
		t.Errorf("expected count 1, got %d", parsed.Count)
	}
}

func TestCheckCmd_DefaultsAndDisabledManagers(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	noRef := true
	cfg := &config.Config{
		DisabledManagers: []string{"mockpm2"},
		Defaults: config.DefaultsConfig{
			Timeout:   "10s",
			NoRefresh: &noRef,
		},
	}
	if err := cfg.SaveTo(tmpCfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	m1 := &mockCheckManager{
		name: "mockpm1",
		updates: []pm.PackageUpdate{
			{Id: "pkg1", Name: "Pkg1", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm1"},
		},
	}
	m2 := &mockCheckManager{
		name: "mockpm2",
		updates: []pm.PackageUpdate{
			{Id: "pkg2", Name: "Pkg2", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm2"},
		},
	}

	reg := pm.NewRegistry(m1, m2)
	reg.SetDisabled(cfg.DisabledManagers)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// Check without args: mockpm2 is skipped
	cmd := NewRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected check to succeed: %v", err)
	}
	var parsed struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if parsed.Count != 1 {
		t.Errorf("expected count 1 (mockpm2 disabled), got %d", parsed.Count)
	}

	// Explicit check on disabled manager should fail with informative error
	cmd2 := NewRootCmd()
	cmd2.SetArgs([]string{"check", "-m", "mockpm2"})
	err := cmd2.Execute()
	if err == nil || !strings.Contains(err.Error(), "disabled in configuration") {
		t.Errorf("expected disabled manager error, got: %v", err)
	}
}
