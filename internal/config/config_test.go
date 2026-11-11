package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestConfig_PinUnpinLoadSave(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "Patchwork", "config.json")

	cfg, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error loading nonexistent config: %v", err)
	}
	if len(cfg.PinnedPackages) != 0 {
		t.Fatalf("expected empty pinned list, got %v", cfg.PinnedPackages)
	}

	if !cfg.Pin("Microsoft.VisualStudioCode") {
		t.Errorf("expected Pin to return true for new package")
	}
	if cfg.Pin("microsoft.visualstudiocode") {
		t.Errorf("expected Pin to return false for duplicate case-insensitive package")
	}
	if !cfg.Pin("Git.Git") {
		t.Errorf("expected Pin to return true for Git.Git")
	}
	if !cfg.Pin("winget:Docker.DockerDesktop") {
		t.Errorf("expected Pin to return true for scoped pin winget:Docker.DockerDesktop")
	}

	if err := cfg.SaveTo(tmpFile); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if !loaded.IsPinned("git.git", "") {
		t.Errorf("expected git.git to be pinned")
	}
	if !loaded.IsPinnedForManager("winget", "Docker.DockerDesktop", "Docker Desktop") {
		t.Errorf("expected winget:Docker.DockerDesktop to match winget manager")
	}
	if loaded.IsPinnedForManager("scoop", "Docker.DockerDesktop", "Docker Desktop") {
		t.Errorf("expected winget:Docker.DockerDesktop NOT to match scoop manager")
	}

	// Name collision across managers: pinning "git" (ID) should not pin winget's "Git.Git" whose display Name is "Git"
	loaded.Pin("git")
	if loaded.IsPinnedForManager("winget", "Git.Git_Other", "Git") {
		t.Errorf("expected unscoped pin 'git' to match on ID only, not display Name")
	}

	if !loaded.Unpin("GIT.GIT") {
		t.Errorf("expected Unpin to return true")
	}
	if loaded.IsPinned("Git.Git", "") {
		t.Errorf("expected Git.Git to be unpinned")
	}
	if !loaded.Unpin("WINGET:docker.dockerdesktop") {
		t.Errorf("expected Unpin to return true for scoped pin")
	}
	if loaded.Unpin("NonExistent") {
		t.Errorf("expected Unpin to return false for nonexistent package")
	}
}

func TestConfig_UpdateCacheSaveLoad(t *testing.T) {
	tmpCache := filepath.Join(t.TempDir(), "sub", "cache.json")
	t.Setenv("PATCHWORK_CACHE", tmpCache)

	if _, ok := LoadUpdateCache(5 * time.Minute); ok {
		t.Fatalf("expected cache miss on nonexistent file")
	}

	sample := []pm.PackageUpdate{
		{Name: "Git", Id: "Git.Git", Version: "2.40.0", AvailableVersion: "2.41.0", ManagerName: "winget"},
	}
	if err := SaveUpdateCache(sample); err != nil {
		t.Fatalf("failed to save update cache: %v", err)
	}

	loaded, ok := LoadUpdateCache(5 * time.Minute)
	if !ok || len(loaded) != 1 || loaded[0].Id != "Git.Git" {
		t.Errorf("expected cache hit with Git.Git, got ok=%v, loaded=%+v", ok, loaded)
	}

	if _, ok := LoadUpdateCache(0); ok {
		t.Errorf("expected cache miss when maxAge is 0")
	}
}

func TestConfig_SaveTo_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		PinnedPackages: []string{"Git.Git"},
	}

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("failed to save config atomically: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("found leftover temporary file from atomic write: %s", e.Name())
		}
	}

	// Overwrite existing file atomically
	cfg.Pin("Microsoft.VisualStudioCode")
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("failed to overwrite config atomically: %v", err)
	}

	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}

	foundConfig := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("found leftover temporary file after overwrite: %s", e.Name())
		}
		if e.Name() == "config.json" {
			foundConfig = true
		}
	}
	if !foundConfig {
		t.Errorf("expected config.json to exist")
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if len(loaded.PinnedPackages) != 2 {
		t.Errorf("expected 2 pinned packages, got %d", len(loaded.PinnedPackages))
	}
}

func TestConfig_LoadFrom_CorruptFailClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write invalid JSON
	if err := os.WriteFile(path, []byte("{ invalid-json-content: true, "), 0600); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	cfg, err := LoadFrom(path)
	if err == nil {
		t.Fatalf("expected error loading corrupted config, got nil error and cfg: %+v", cfg)
	}
	if cfg != nil {
		t.Errorf("expected nil config on error, got: %+v", cfg)
	}
}

func TestConfig_AcceptAgreementsPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Default should be true
	var emptyCfg *Config
	if !emptyCfg.ShouldAcceptAgreements() {
		t.Errorf("expected nil config ShouldAcceptAgreements to be true")
	}

	cfg1 := &Config{}
	if !cfg1.ShouldAcceptAgreements() {
		t.Errorf("expected unspecified AcceptAgreements to default to true")
	}

	// Set to false
	fVal := false
	cfg1.AcceptAgreements = &fVal
	if err := cfg1.SaveTo(path); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.ShouldAcceptAgreements() {
		t.Errorf("expected loaded config ShouldAcceptAgreements to be false")
	}

	// Set to true
	tVal := true
	loaded.AcceptAgreements = &tVal
	if err := loaded.SaveTo(path); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !loaded2.ShouldAcceptAgreements() {
		t.Errorf("expected loaded config ShouldAcceptAgreements to be true")
	}
}

func TestConfig_UnmarshalFormats(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Legacy string array format
	legacyData := []byte(`{
		"pinned_packages": ["git", "winget:Docker.DockerDesktop"]
	}`)
	path1 := filepath.Join(tmpDir, "legacy.json")
	if err := os.WriteFile(path1, legacyData, 0600); err != nil {
		t.Fatal(err)
	}
	cfg1, err := LoadFrom(path1)
	if err != nil {
		t.Fatalf("failed to load legacy config: %v", err)
	}
	if len(cfg1.Pins) != 2 || len(cfg1.PinnedPackages) != 2 {
		t.Errorf("expected 2 pins, got pins=%d, pinned_packages=%d", len(cfg1.Pins), len(cfg1.PinnedPackages))
	}
	if !cfg1.IsPinnedForManager("winget", "Docker.DockerDesktop", "") {
		t.Errorf("expected winget:Docker.DockerDesktop to be pinned")
	}

	// 2. Rich pins array format
	exp := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	richData := []byte(fmt.Sprintf(`{
		"pins": [
			{"id": "node", "version": "<23", "reason": "driver break", "expires_at": %q},
			{"id": "curl", "manager": "winget"}
		],
		"disabled_managers": ["cargo", "psresource"],
		"defaults": {
			"timeout": "45s",
			"no_refresh": true
		}
	}`, exp.Format(time.RFC3339)))
	path2 := filepath.Join(tmpDir, "rich.json")
	if err := os.WriteFile(path2, richData, 0600); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadFrom(path2)
	if err != nil {
		t.Fatalf("failed to load rich config: %v", err)
	}
	if len(cfg2.Pins) != 2 {
		t.Fatalf("expected 2 pins, got %d", len(cfg2.Pins))
	}
	if cfg2.Pins[0].Version != "<23" || cfg2.Pins[0].Reason != "driver break" {
		t.Errorf("expected rich pin attributes preserved, got %+v", cfg2.Pins[0])
	}
	if len(cfg2.DisabledManagers) != 2 || cfg2.DisabledManagers[0] != "cargo" {
		t.Errorf("expected disabled_managers preserved, got %v", cfg2.DisabledManagers)
	}
	if cfg2.Defaults.Timeout != "45s" || cfg2.Defaults.NoRefresh == nil || !*cfg2.Defaults.NoRefresh {
		t.Errorf("expected defaults preserved, got %+v", cfg2.Defaults)
	}

	// 3. Map format for pins
	mapData := []byte(`{
		"pins": {
			"python": {"version": "~3.11"},
			"winget:vlc": {"reason": "stable"}
		}
	}`)
	path3 := filepath.Join(tmpDir, "map.json")
	if err := os.WriteFile(path3, mapData, 0600); err != nil {
		t.Fatal(err)
	}
	cfg3, err := LoadFrom(path3)
	if err != nil {
		t.Fatalf("failed to load map format config: %v", err)
	}
	if len(cfg3.Pins) != 2 {
		t.Fatalf("expected 2 pins from map format, got %d", len(cfg3.Pins))
	}
	pin, violated := cfg3.MatchPin("", "python", "python", "3.11.5")
	if pin == nil || violated {
		t.Errorf("expected python 3.11.5 to satisfy ~3.11 constraint (not violated)")
	}
	_, violated = cfg3.MatchPin("", "python", "python", "3.12.0")
	if !violated {
		t.Errorf("expected python 3.12.0 to violate ~3.11 constraint (violated)")
	}
}

func TestConfig_MatchPin_ConstraintsAndExpiry(t *testing.T) {
	now := time.Now()
	expired := now.Add(-1 * time.Hour)
	future := now.Add(24 * time.Hour)

	cfg := &Config{}
	// Unconditional pin
	cfg.AddPinEntry(PinEntry{ID: "git"})
	// Scoped pin with constraint
	cfg.AddPinEntry(PinEntry{Manager: "npm", ID: "typescript", Version: "<5.0.0"})
	// Expired pin
	cfg.AddPinEntry(PinEntry{ID: "legacy-pkg", ExpiresAt: &expired})
	// Active future expiry pin
	cfg.AddPinEntry(PinEntry{ID: "temp-pkg", ExpiresAt: &future})

	// 1. Unconditional pin
	pin, violated := cfg.MatchPin("", "git", "git", "2.40.0")
	if pin == nil || !violated {
		t.Errorf("expected git to be unconditionally pinned and violated")
	}

	// 2. Version constraint satisfied -> not violated
	pin, violated = cfg.MatchPin("npm", "typescript", "typescript", "4.9.5")
	if pin == nil || violated {
		t.Errorf("expected typescript 4.9.5 to satisfy <5.0.0 (violated=false)")
	}

	// 3. Version constraint violated -> violated
	pin, violated = cfg.MatchPin("npm", "typescript", "typescript", "5.1.0")
	if pin == nil || !violated {
		t.Errorf("expected typescript 5.1.0 to violate <5.0.0 (violated=true)")
	}

	// 4. Different manager for scoped pin -> not matched
	pin, violated = cfg.MatchPin("cargo", "typescript", "typescript", "5.1.0")
	if pin != nil || violated {
		t.Errorf("expected npm:typescript not to match cargo manager")
	}

	// 5. Expired pin -> active=false, violated=false
	pin, violated = cfg.MatchPin("", "legacy-pkg", "legacy-pkg", "1.0.0")
	if pin == nil || violated {
		t.Errorf("expected expired pin not to violate upgrade (violated=false)")
	}
	if !pin.IsExpired() {
		t.Errorf("expected pin.IsExpired() to return true")
	}

	// 6. Non-expired pin -> violated=true
	pin, violated = cfg.MatchPin("", "temp-pkg", "temp-pkg", "2.0.0")
	if pin == nil || !violated {
		t.Errorf("expected active unexpired pin to be violated")
	}
}

func TestConfig_GetSetValidation(t *testing.T) {
	cfg := &Config{}

	// Test defaults.timeout
	if err := cfg.Set("defaults.timeout", "30s"); err != nil {
		t.Fatalf("unexpected error setting defaults.timeout: %v", err)
	}
	val, err := cfg.Get("defaults.timeout")
	if err != nil || val != "30s" {
		t.Errorf("expected defaults.timeout=30s, got val=%v, err=%v", val, err)
	}
	// Invalid duration
	if err := cfg.Set("defaults.timeout", "invalid-duration"); err == nil {
		t.Errorf("expected error setting invalid duration")
	}

	// Test boolean flags
	if err := cfg.Set("defaults.accept-agreements", "true"); err != nil {
		t.Fatalf("unexpected error setting defaults.accept-agreements: %v", err)
	}
	val, err = cfg.Get("defaults.accept_agreements")
	if err != nil || val != true {
		t.Errorf("expected defaults.accept_agreements=true, got %v", val)
	}
	if err := cfg.Set("defaults.accept-agreements", "invalid-bool"); err == nil {
		t.Errorf("expected error setting invalid bool")
	}

	if err := cfg.Set("defaults.trust_repository", true); err != nil {
		t.Fatalf("unexpected error setting defaults.trust_repository: %v", err)
	}
	val, _ = cfg.Get("defaults.trust_repository")
	if val != true {
		t.Errorf("expected defaults.trust_repository=true, got %v", val)
	}

	if err := cfg.Set("defaults.no_refresh", true); err != nil {
		t.Fatalf("unexpected error setting defaults.no_refresh: %v", err)
	}
	val, _ = cfg.Get("defaults.no_refresh")
	if val != true {
		t.Errorf("expected defaults.no_refresh=true, got %v", val)
	}

	// Test disabled_managers
	if err := cfg.Set("disabled_managers", "cargo,psresource,apt,dnf,pacman,zypper,flatpak,snap,mas,pipx,gem,pnpm,bun,rustup,mise,asdf,vscode"); err != nil {
		t.Fatalf("unexpected error setting disabled_managers: %v", err)
	}
	val, _ = cfg.Get("disabled_managers")
	list, ok := val.([]string)
	if !ok || len(list) != 17 || list[0] != "cargo" || list[1] != "psresource" {
		t.Errorf("expected 17 disabled managers, got %v", val)
	}
	if err := cfg.Set("disabled_managers", "unknown-manager"); err == nil {
		t.Errorf("expected error setting unknown manager name")
	}

	// Test unknown key
	if err := cfg.Set("invalid.key", "val"); err == nil {
		t.Errorf("expected error setting unknown key")
	}
	if _, err := cfg.Get("invalid.key"); err == nil {
		t.Errorf("expected error getting unknown key")
	}
}
