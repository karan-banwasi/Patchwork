package config

import (
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
