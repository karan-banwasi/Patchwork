package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

// DefaultsConfig represents default command-line flags configured persistently by the user.
type DefaultsConfig struct {
	Timeout          string `json:"timeout,omitempty"`
	AcceptAgreements *bool  `json:"accept_agreements,omitempty"`
	TrustRepository  *bool  `json:"trust_repository,omitempty"`
	NoRefresh        *bool  `json:"no_refresh,omitempty"`
}

// PinEntry represents a pinned package with optional version constraints, reason, and expiry.
type PinEntry struct {
	ID        string     `json:"id"`
	Manager   string     `json:"manager,omitempty"`
	Version   string     `json:"version,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsExpired reports whether the pin has passed its expiration timestamp.
func (p *PinEntry) IsExpired() bool {
	if p == nil || p.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*p.ExpiresAt)
}

// UnmarshalJSON unmarshals a PinEntry from either a JSON string ("git" or "winget:git") or an object.
func (p *PinEntry) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		norm := normalizePinEntry(s)
		if idx := strings.IndexByte(norm, ':'); idx > 0 && idx < len(norm)-1 {
			p.Manager = norm[:idx]
			p.ID = norm[idx+1:]
		} else {
			p.ID = norm
		}
		return nil
	}

	type rawPin PinEntry
	var raw rawPin
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = PinEntry(raw)
	if p.Manager == "" && strings.Contains(p.ID, ":") {
		norm := normalizePinEntry(p.ID)
		if idx := strings.IndexByte(norm, ':'); idx > 0 && idx < len(norm)-1 {
			p.Manager = norm[:idx]
			p.ID = norm[idx+1:]
		}
	}
	return nil
}

// Config represents the persistent user configuration for Patchwork.
type Config struct {
	PinnedPackages   []string       `json:"pinned_packages,omitempty"`
	Pins             []PinEntry     `json:"pins,omitempty"`
	DisabledManagers []string       `json:"disabled_managers,omitempty"`
	Defaults         DefaultsConfig `json:"defaults,omitempty"`
	AcceptAgreements *bool          `json:"accept_agreements,omitempty"`
}

// ShouldAcceptAgreements returns whether package manager agreements should be automatically accepted.
// Defaults to true when unspecified.
func (c *Config) ShouldAcceptAgreements() bool {
	if c == nil {
		return true
	}
	if c.Defaults.AcceptAgreements != nil {
		return *c.Defaults.AcceptAgreements
	}
	if c.AcceptAgreements != nil {
		return *c.AcceptAgreements
	}
	return true
}

func (c *Config) syncPinsFromPackages() {
	if c == nil {
		return
	}
	pinMap := make(map[string]bool)
	for _, p := range c.Pins {
		key := strings.ToLower(p.ID)
		if p.Manager != "" {
			key = strings.ToLower(p.Manager) + ":" + strings.ToLower(p.ID)
		}
		pinMap[key] = true
	}
	for _, s := range c.PinnedPackages {
		norm := normalizePinEntry(s)
		key := strings.ToLower(norm)
		if !pinMap[key] {
			var entry PinEntry
			if idx := strings.IndexByte(norm, ':'); idx > 0 && idx < len(norm)-1 {
				entry.Manager = norm[:idx]
				entry.ID = norm[idx+1:]
			} else {
				entry.ID = norm
			}
			c.Pins = append(c.Pins, entry)
			pinMap[key] = true
		}
	}
}

func (c *Config) syncPackagesFromPins() {
	if c == nil {
		return
	}
	c.PinnedPackages = make([]string, 0, len(c.Pins))
	for _, p := range c.Pins {
		var s string
		if p.Manager != "" {
			s = strings.ToLower(p.Manager) + ":" + p.ID
		} else {
			s = p.ID
		}
		c.PinnedPackages = append(c.PinnedPackages, s)
	}
	sort.Strings(c.PinnedPackages)
}

func (c *Config) hasRichPins() bool {
	if c == nil {
		return false
	}
	for _, p := range c.Pins {
		if p.Version != "" || p.Reason != "" || p.ExpiresAt != nil {
			return true
		}
	}
	return false
}

// UnmarshalJSON deserializes configuration supporting legacy string arrays, object lists, or map format.
func (c *Config) UnmarshalJSON(data []byte) error {
	type rawCfg struct {
		PinnedPackages   json.RawMessage `json:"pinned_packages"`
		Pins             json.RawMessage `json:"pins"`
		DisabledManagers []string        `json:"disabled_managers"`
		Defaults         DefaultsConfig  `json:"defaults"`
		AcceptAgreements *bool           `json:"accept_agreements"`
	}
	var raw rawCfg
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.DisabledManagers = raw.DisabledManagers
	c.Defaults = raw.Defaults
	c.AcceptAgreements = raw.AcceptAgreements

	pinMap := make(map[string]PinEntry)
	var orderedKeys []string

	addOrUpdatePin := func(entry PinEntry) {
		normID := normalizePinEntry(entry.ID)
		if entry.Manager == "" && strings.Contains(normID, ":") {
			if idx := strings.IndexByte(normID, ':'); idx > 0 && idx < len(normID)-1 {
				entry.Manager = normID[:idx]
				entry.ID = normID[idx+1:]
			}
		}
		key := strings.ToLower(entry.ID)
		if entry.Manager != "" {
			key = strings.ToLower(entry.Manager) + ":" + strings.ToLower(entry.ID)
		}
		existing, found := pinMap[key]
		if !found {
			orderedKeys = append(orderedKeys, key)
			pinMap[key] = entry
		} else {
			if entry.Version != "" {
				existing.Version = entry.Version
			}
			if entry.Reason != "" {
				existing.Reason = entry.Reason
			}
			if entry.ExpiresAt != nil {
				existing.ExpiresAt = entry.ExpiresAt
			}
			if entry.Manager != "" {
				existing.Manager = entry.Manager
			}
			pinMap[key] = existing
		}
	}

	// 1. Parse pinned_packages if present
	if len(raw.PinnedPackages) > 0 && string(raw.PinnedPackages) != "null" {
		var strList []string
		if err := json.Unmarshal(raw.PinnedPackages, &strList); err == nil {
			for _, s := range strList {
				var p PinEntry
				_ = p.UnmarshalJSON([]byte(fmt.Sprintf("%q", s)))
				addOrUpdatePin(p)
			}
		} else {
			var pinList []PinEntry
			if err := json.Unmarshal(raw.PinnedPackages, &pinList); err != nil {
				return fmt.Errorf("invalid pinned_packages format: %w", err)
			}
			for _, p := range pinList {
				addOrUpdatePin(p)
			}
		}
	}

	// 2. Parse pins if present
	if len(raw.Pins) > 0 && string(raw.Pins) != "null" {
		var pinList []PinEntry
		if err := json.Unmarshal(raw.Pins, &pinList); err == nil {
			for _, p := range pinList {
				addOrUpdatePin(p)
			}
		} else {
			var pinObjMap map[string]json.RawMessage
			if mapErr := json.Unmarshal(raw.Pins, &pinObjMap); mapErr == nil {
				for k, v := range pinObjMap {
					var p PinEntry
					if err := json.Unmarshal(v, &p); err == nil {
						if p.ID == "" {
							p.ID = k
						}
						addOrUpdatePin(p)
					}
				}
			} else {
				return fmt.Errorf("invalid pins format: %w", err)
			}
		}
	}

	c.Pins = make([]PinEntry, 0, len(orderedKeys))
	c.PinnedPackages = make([]string, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		p := pinMap[k]
		c.Pins = append(c.Pins, p)
		var strID string
		if p.Manager != "" {
			strID = strings.ToLower(p.Manager) + ":" + p.ID
		} else {
			strID = p.ID
		}
		c.PinnedPackages = append(c.PinnedPackages, strID)
	}
	sort.Strings(c.PinnedPackages)
	return nil
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
	cfg.syncPinsFromPackages()
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

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true

	if err := os.Chmod(tmpName, perm); err != nil {
		// Non-fatal on Windows
	}
	return os.Rename(tmpName, path)
}

// SaveTo writes the configuration to a specific path with user-only permissions (0700/0600) atomically.
func (c *Config) SaveTo(path string) error {
	c.syncPinsFromPackages()
	c.syncPackagesFromPins()

	type configJSON struct {
		PinnedPackages   []string        `json:"pinned_packages,omitempty"`
		Pins             []PinEntry      `json:"pins,omitempty"`
		DisabledManagers []string        `json:"disabled_managers,omitempty"`
		Defaults         *DefaultsConfig `json:"defaults,omitempty"`
		AcceptAgreements *bool           `json:"accept_agreements,omitempty"`
	}

	out := configJSON{
		PinnedPackages:   c.PinnedPackages,
		DisabledManagers: c.DisabledManagers,
		AcceptAgreements: c.AcceptAgreements,
	}
	if c.Defaults.Timeout != "" || c.Defaults.AcceptAgreements != nil || c.Defaults.TrustRepository != nil || c.Defaults.NoRefresh != nil {
		out.Defaults = &c.Defaults
	}
	if c.hasRichPins() {
		out.Pins = c.Pins
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0600)
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

// AddPinEntry adds or updates a PinEntry with rich attributes (version constraint, reason, expiry).
func (c *Config) AddPinEntry(entry PinEntry) bool {
	c.syncPinsFromPackages()
	normID := normalizePinEntry(entry.ID)
	if entry.Manager == "" && strings.Contains(normID, ":") {
		if idx := strings.IndexByte(normID, ':'); idx > 0 && idx < len(normID)-1 {
			entry.Manager = normID[:idx]
			entry.ID = normID[idx+1:]
		}
	} else {
		entry.ID = normID
	}
	if entry.ID == "" {
		return false
	}

	targetMgr := strings.ToLower(strings.TrimSpace(entry.Manager))
	targetID := strings.TrimSpace(entry.ID)

	for i, existing := range c.Pins {
		existingMgr := strings.ToLower(strings.TrimSpace(existing.Manager))
		existingID := strings.TrimSpace(existing.ID)
		if strings.EqualFold(existingMgr, targetMgr) && strings.EqualFold(existingID, targetID) {
			c.Pins[i] = entry
			c.syncPackagesFromPins()
			return true
		}
	}

	c.Pins = append(c.Pins, entry)
	c.syncPackagesFromPins()
	return true
}

// AddPin adds a package ID to the pinned list. Alias for Pin.
func (c *Config) AddPin(id string) bool {
	return c.Pin(id)
}

// Pin adds a package ID (optionally scoped as "manager:id") to the pinned list if not already present.
// Returns true if newly pinned.
func (c *Config) Pin(id string) bool {
	c.syncPinsFromPackages()
	norm := normalizePinEntry(id)
	if norm == "" {
		return false
	}
	var entry PinEntry
	if idx := strings.IndexByte(norm, ':'); idx > 0 && idx < len(norm)-1 {
		entry.Manager = norm[:idx]
		entry.ID = norm[idx+1:]
	} else {
		entry.ID = norm
	}

	targetMgr := strings.ToLower(entry.Manager)
	targetID := entry.ID

	for _, existing := range c.Pins {
		existingMgr := strings.ToLower(existing.Manager)
		if strings.EqualFold(existingMgr, targetMgr) && strings.EqualFold(existing.ID, targetID) {
			return false
		}
	}

	c.Pins = append(c.Pins, entry)
	c.syncPackagesFromPins()
	return true
}

// RemovePin removes a package ID from the pinned list. Alias for Unpin.
func (c *Config) RemovePin(id string) bool {
	return c.Unpin(id)
}

// Unpin removes a package ID (or "manager:id") from the pinned list. Returns true if it was present and removed.
func (c *Config) Unpin(id string) bool {
	c.syncPinsFromPackages()
	norm := normalizePinEntry(id)
	if norm == "" {
		return false
	}
	var targetMgr string
	targetID := norm
	if idx := strings.IndexByte(norm, ':'); idx > 0 && idx < len(norm)-1 {
		targetMgr = norm[:idx]
		targetID = norm[idx+1:]
	}

	var kept []PinEntry
	removed := false
	for _, p := range c.Pins {
		if strings.EqualFold(p.ID, targetID) {
			if targetMgr == "" || strings.EqualFold(p.Manager, targetMgr) {
				removed = true
				continue
			}
		}
		if targetMgr != "" && strings.EqualFold(p.Manager+":"+p.ID, norm) {
			removed = true
			continue
		}
		kept = append(kept, p)
	}
	if removed {
		c.Pins = kept
		c.syncPackagesFromPins()
	}
	return removed
}

// MatchPin evaluates whether the specified package matches any pin in configuration,
// checking manager scoping, expiration, and version constraints.
// Returns the matched PinEntry (if any) and whether the pin is active and violated by targetVersion.
func (c *Config) MatchPin(manager, id, name, targetVersion string) (*PinEntry, bool) {
	if c == nil || id == "" {
		return nil, false
	}
	c.syncPinsFromPackages()
	mgrNorm := strings.ToLower(strings.TrimSpace(manager))
	idNorm := strings.TrimSpace(id)

	for i := range c.Pins {
		pin := &c.Pins[i]
		pinMgr := strings.ToLower(strings.TrimSpace(pin.Manager))
		pinID := strings.TrimSpace(pin.ID)

		matched := false
		if pinMgr != "" {
			if mgrNorm != "" && strings.EqualFold(pinMgr, mgrNorm) && strings.EqualFold(pinID, idNorm) {
				matched = true
			} else if strings.EqualFold(pinMgr+":"+pinID, idNorm) {
				matched = true
			}
		} else {
			if strings.EqualFold(pinID, idNorm) {
				matched = true
			} else if mgrNorm != "" && strings.EqualFold(mgrNorm+":"+pinID, idNorm) {
				matched = true
			}
		}

		if matched {
			if pin.IsExpired() {
				return pin, false
			}
			if pin.Version != "" {
				if targetVersion == "" {
					return pin, true
				}
				satisfies, err := version.EvaluateConstraint(manager, targetVersion, pin.Version)
				if err != nil {
					return pin, true
				}
				if satisfies {
					return pin, false
				}
				return pin, true
			}
			return pin, true
		}
	}
	return nil, false
}

// IsPinned returns true if an unscoped package ID matches the pinned list.
func (c *Config) IsPinned(id, name string) bool {
	return c.IsPinnedForManager("", id, name)
}

// IsPinnedForManager returns true if the given package (identified by manager and id) is pinned.
func (c *Config) IsPinnedForManager(manager, id, name string) bool {
	if c == nil || id == "" {
		return false
	}
	c.syncPinsFromPackages()
	_, violated := c.MatchPin(manager, id, name, "")
	return violated
}

// SupportedKeys lists the user-configurable configuration keys.
var SupportedKeys = []string{
	"defaults.timeout",
	"defaults.accept_agreements",
	"defaults.trust_repository",
	"defaults.no_refresh",
	"disabled_managers",
	"accept_agreements",
	"pins",
	"pinned_packages",
}

func normalizeConfigKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, "-", "_")
	return k
}

// Get retrieves the value of a configuration key supporting dotted paths.
func (c *Config) Get(key string) (any, error) {
	if c == nil {
		return nil, errors.New("config is nil")
	}
	k := normalizeConfigKey(key)
	switch k {
	case "defaults.timeout":
		return c.Defaults.Timeout, nil
	case "defaults.accept_agreements":
		if c.Defaults.AcceptAgreements != nil {
			return *c.Defaults.AcceptAgreements, nil
		}
		return nil, nil
	case "defaults.trust_repository":
		if c.Defaults.TrustRepository != nil {
			return *c.Defaults.TrustRepository, nil
		}
		return nil, nil
	case "defaults.no_refresh":
		if c.Defaults.NoRefresh != nil {
			return *c.Defaults.NoRefresh, nil
		}
		return nil, nil
	case "defaults":
		return c.Defaults, nil
	case "disabled_managers":
		return c.DisabledManagers, nil
	case "accept_agreements":
		if c.AcceptAgreements != nil {
			return *c.AcceptAgreements, nil
		}
		return nil, nil
	case "pins":
		c.syncPinsFromPackages()
		return c.Pins, nil
	case "pinned_packages":
		c.syncPackagesFromPins()
		return c.PinnedPackages, nil
	default:
		return nil, fmt.Errorf("unknown configuration key %q (supported: %s)", key, strings.Join(SupportedKeys, ", "))
	}
}

func parseBoolVal(val any) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		vTrim := strings.ToLower(strings.TrimSpace(v))
		switch vTrim {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean value %q", v)
		}
	default:
		return false, fmt.Errorf("expected boolean, got %T", val)
	}
}

// Set sets the value of a configuration key with validation, supporting dotted paths.
func (c *Config) Set(key string, val any) error {
	if c == nil {
		return errors.New("config is nil")
	}
	k := normalizeConfigKey(key)
	switch k {
	case "defaults.timeout":
		str := fmt.Sprint(val)
		str = strings.TrimSpace(str)
		if str == "" || str == "0" {
			c.Defaults.Timeout = ""
			return nil
		}
		if _, err := time.ParseDuration(str); err != nil {
			return fmt.Errorf("invalid duration %q for defaults.timeout: %w", str, err)
		}
		c.Defaults.Timeout = str
		return nil

	case "defaults.accept_agreements":
		b, err := parseBoolVal(val)
		if err != nil {
			return fmt.Errorf("invalid value for defaults.accept_agreements: %w", err)
		}
		c.Defaults.AcceptAgreements = &b
		return nil

	case "defaults.trust_repository":
		b, err := parseBoolVal(val)
		if err != nil {
			return fmt.Errorf("invalid value for defaults.trust_repository: %w", err)
		}
		c.Defaults.TrustRepository = &b
		return nil

	case "defaults.no_refresh":
		b, err := parseBoolVal(val)
		if err != nil {
			return fmt.Errorf("invalid value for defaults.no_refresh: %w", err)
		}
		c.Defaults.NoRefresh = &b
		return nil

	case "accept_agreements":
		b, err := parseBoolVal(val)
		if err != nil {
			return fmt.Errorf("invalid value for accept_agreements: %w", err)
		}
		c.AcceptAgreements = &b
		return nil

	case "disabled_managers":
		var list []string
		switch v := val.(type) {
		case []string:
			list = v
		case string:
			if strings.TrimSpace(v) != "" {
				for _, part := range strings.Split(v, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						list = append(list, part)
					}
				}
			}
		default:
			return fmt.Errorf("invalid disabled_managers value: expected string or list of strings, got %T", val)
		}

		knownManagers := map[string]bool{
			// Existing
			"brew": true, "homebrew": true,
			"winget": true,
			"scoop":  true,
			"choco":  true, "chocolatey": true,
			"psresource": true, "powershell": true,
			"npm":    true,
			"dotnet": true,
			"uv":     true, "python": true,
			"cargo": true, "rust": true,

			// Linux-native
			"apt": true, "apt-get": true, "debian": true, "ubuntu": true,
			"dnf": true, "yum": true, "fedora": true, "rhel": true,
			"pacman": true, "arch": true,
			"zypper": true, "opensuse": true, "suse": true,
			"flatpak": true, "flathub": true,
			"snap": true, "snapcraft": true, "snappy": true,

			// macOS-native
			"mas": true, "macappstore": true, "appstore": true,

			// Cross-platform
			"pipx": true, "pipx-app": true,
			"gem": true, "rubygems": true, "ruby": true,
			"pnpm": true, "pnpm-global": true,
			"bun": true, "bun-global": true,
			"rustup": true, "rust-toolchain": true,
			"mise": true, "rtx": true,
			"asdf":   true,
			"vscode": true, "code": true, "vs-code": true,
		}
		var clean []string
		for _, m := range list {
			norm := strings.ToLower(strings.TrimSpace(m))
			if norm == "" {
				continue
			}
			if !knownManagers[norm] {
				return fmt.Errorf("unknown package manager %q", m)
			}
			clean = append(clean, norm)
		}
		c.DisabledManagers = clean
		return nil

	case "pinned_packages":
		var list []string
		switch v := val.(type) {
		case []string:
			list = v
		case string:
			if strings.TrimSpace(v) != "" {
				for _, part := range strings.Split(v, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						list = append(list, part)
					}
				}
			}
		default:
			return fmt.Errorf("invalid pinned_packages value: expected string or list of strings, got %T", val)
		}
		c.Pins = nil
		c.PinnedPackages = nil
		for _, item := range list {
			c.Pin(item)
		}
		return nil

	default:
		return fmt.Errorf("unknown configuration key %q (supported: %s)", key, strings.Join(SupportedKeys, ", "))
	}
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

// SaveUpdateCache writes the latest update check results to the cache file with 0700/0600 permissions atomically.
func SaveUpdateCache(updates []pm.PackageUpdate) error {
	cache := UpdateCache{
		UpdatedAt: time.Now(),
		Updates:   updates,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return writeAtomic(CachePath(), data, 0600)
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
