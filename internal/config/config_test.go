package config

import (
	"path/filepath"
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
	if !loaded.IsPinned("other-id", "Microsoft.VisualStudioCode") {
		t.Errorf("expected match by name to be pinned")
	}

	if !loaded.Unpin("GIT.GIT") {
		t.Errorf("expected Unpin to return true")
	}
	if loaded.IsPinned("Git.Git", "") {
		t.Errorf("expected Git.Git to be unpinned")
	}
	if loaded.Unpin("NonExistent") {
		t.Errorf("expected Unpin to return false for nonexistent package")
	}
}

func TestConfig_UpdateCacheSaveLoad(t *testing.T) {
	tmpCache := filepath.Join(t.TempDir(), "cache.json")
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
