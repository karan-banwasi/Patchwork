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

// SaveTo writes the configuration to a specific path with user-only permissions (0700/0600).
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func normalizePinEntry(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if idx := strings.IndexByte(trimmed, ':'); idx > 0 && idx < len(trimmed)-1 {
		mgr := strings.ToLower(strings.TrimSpace(trimmed[:idx]))
		id := strings.TrimSpace(trimmed[idx+1:])
		if mgr != "" && id != "" {
			return mgr + ":" + id
		}
	}
	return trimmed
}

// Pin adds a package ID (optionally scoped as "manager:id") to the pinned list if not already present.
// Returns true if newly pinned.
func (c *Config) Pin(id string) bool {
	norm := normalizePinEntry(id)
	if norm == "" {
		return false
	}
	for _, existing := range c.PinnedPackages {
		if strings.EqualFold(existing, norm) {
			return false
		}
	}
	c.PinnedPackages = append(c.PinnedPackages, norm)
	sort.Strings(c.PinnedPackages)
	return true
}

// Unpin removes a package ID (or "manager:id") from the pinned list. Returns true if it was present and removed.
func (c *Config) Unpin(id string) bool {
	norm := normalizePinEntry(id)
	var kept []string
	removed := false
	for _, p := range c.PinnedPackages {
		if strings.EqualFold(p, norm) {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	c.PinnedPackages = kept
	return removed
}

// IsPinned returns true if an unscoped package ID matches the pinned list.
func (c *Config) IsPinned(id, name string) bool {
	return c.IsPinnedForManager("", id, name)
}

// IsPinnedForManager returns true if the given package (identified by manager and id) is pinned.
// Entries formatted as "manager:id" only match that specific manager; unscoped entries match any manager by package ID.
func (c *Config) IsPinnedForManager(manager, id, _ string) bool {
	if c == nil || id == "" {
		return false
	}
	mgrNorm := strings.ToLower(strings.TrimSpace(manager))
	for _, p := range c.PinnedPackages {
		if idx := strings.IndexByte(p, ':'); idx > 0 && idx < len(p)-1 {
			pinMgr := strings.ToLower(strings.TrimSpace(p[:idx]))
			pinID := strings.TrimSpace(p[idx+1:])
			if mgrNorm != "" && strings.EqualFold(pinMgr, mgrNorm) && strings.EqualFold(pinID, id) {
				return true
			}
			// Also allow exact match when caller passes "manager:id" as id
			if strings.EqualFold(p, id) {
				return true
			}
			continue
		}
		if strings.EqualFold(p, id) {
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

// CachePath returns the path to the user-scoped update cache file.
func CachePath() string {
	if custom := os.Getenv("PATCHWORK_CACHE"); custom != "" {
		return custom
	}
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "Patchwork", "updates-cache.json")
	}
	return filepath.Join(os.TempDir(), "patchwork-updates-cache.json")
}

// SaveUpdateCache writes the latest update check results to the cache file with 0700/0600 permissions.
func SaveUpdateCache(updates []pm.PackageUpdate) error {
	cache := UpdateCache{
		UpdatedAt: time.Now(),
		Updates:   updates,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	path := CachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
