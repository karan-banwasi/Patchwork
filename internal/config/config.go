package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// Config represents the persistent user configuration for Patchwork.
type Config struct {
	PinnedPackages []string `json:"pinned_packages"`
}

// ConfigPath returns the path to the Patchwork config file.
func ConfigPath() (string, error) {
	if custom := os.Getenv("PATCHWORK_CONFIG"); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Patchwork", "config.json"), nil
}

// Load reads the configuration from disk, returning an empty Config if it does not exist.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return &Config{}, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the configuration from a specific path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the configuration to the default config path.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo writes the configuration to a specific path.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Pin adds a package ID to the pinned list if not already present. Returns true if newly pinned.
func (c *Config) Pin(id string) bool {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || c.IsPinned(trimmed, "") {
		return false
	}
	c.PinnedPackages = append(c.PinnedPackages, trimmed)
	sort.Strings(c.PinnedPackages)
	return true
}

// Unpin removes a package ID from the pinned list. Returns true if it was present and removed.
func (c *Config) Unpin(id string) bool {
	trimmed := strings.TrimSpace(id)
	var kept []string
	removed := false
	for _, p := range c.PinnedPackages {
		if strings.EqualFold(p, trimmed) {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	c.PinnedPackages = kept
	return removed
}

// IsPinned returns true if a package ID or Name is in the pinned list.
func (c *Config) IsPinned(id, name string) bool {
	if c == nil {
		return false
	}
	for _, p := range c.PinnedPackages {
		if strings.EqualFold(p, id) || (name != "" && strings.EqualFold(p, name)) {
			return true
		}
	}
	return false
}

// UpdateCache represents a short-lived cache of available package updates for fast shell completions.
type UpdateCache struct {
	UpdatedAt time.Time          `json:"updated_at"`
	Updates   []pm.PackageUpdate `json:"updates"`
}

// CachePath returns the path to the temporary update cache file.
func CachePath() string {
	if custom := os.Getenv("PATCHWORK_CACHE"); custom != "" {
		return custom
	}
	return filepath.Join(os.TempDir(), "patchwork-updates-cache.json")
}

// SaveUpdateCache writes the latest update check results to the temporary cache file.
func SaveUpdateCache(updates []pm.PackageUpdate) error {
	cache := UpdateCache{
		UpdatedAt: time.Now(),
		Updates:   updates,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(CachePath(), data, 0600)
}

// LoadUpdateCache reads cached package updates if the cache file exists and is newer than maxAge.
func LoadUpdateCache(maxAge time.Duration) ([]pm.PackageUpdate, bool) {
	data, err := os.ReadFile(CachePath())
	if err != nil {
		return nil, false
	}
	var cache UpdateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if time.Since(cache.UpdatedAt) > maxAge {
		return nil, false
	}
	return cache.Updates, true
}
