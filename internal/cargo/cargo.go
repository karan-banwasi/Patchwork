package cargo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// CargoManager implements the pm.Manager interface for Rust global binaries installed via `cargo install`.
type CargoManager struct{}

func NewCargoManager() *CargoManager {
	return &CargoManager{}
}

func (c *CargoManager) Name() string {
	return "cargo"
}

func (c *CargoManager) IsAvailable(ctx context.Context) bool {
	_, err := cargoPath()
	return err == nil
}

func cargoPath() (string, error) {
	for _, name := range []string{"cargo.exe", "cargo"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("cargo executable not found")
}

// Example header line in `cargo install --list`:
// ripgrep v14.1.0:
// stylua v0.20.0 (https://github.com/...): -> local/git crates have a parenthesized source and should be skipped
var cargoCrateLineRegex = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s+v([0-9][^\s:()]*):$`)

func parseCargoInstallList(output string) []pm.PackageUpdate {
	var crates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n ")
		if trimmed == "" || strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") {
			continue
		}
		m := cargoCrateLineRegex.FindStringSubmatch(trimmed)
		if len(m) == 3 {
			crates = append(crates, pm.PackageUpdate{
				Name:        m[1],
				Id:          m[1],
				Version:     m[2],
				Source:      "crates.io",
				ManagerName: "cargo",
			})
		}
	}
	return crates
}

type cratesIOResponse struct {
	Crate struct {
		MaxStableVersion string `json:"max_stable_version"`
		NewestVersion    string `json:"newest_version"`
	} `json:"crate"`
}

func fetchCratesIOLatestVersion(ctx context.Context, client *http.Client, crateName string) string {
	url := fmt.Sprintf("https://crates.io/api/v1/crates/%s", crateName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Patchwork-CLI (https://github.com/karan-banwasi/Patchwork)")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data cratesIOResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	if data.Crate.MaxStableVersion != "" {
		return strings.TrimSpace(data.Crate.MaxStableVersion)
	}
	return strings.TrimSpace(data.Crate.NewestVersion)
}

func (c *CargoManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := cargoPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, exe, "install", "--list")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return []pm.PackageUpdate{}, nil
	}

	crates := parseCargoInstallList(stdout.String())
	if len(crates) == 0 {
		return []pm.PackageUpdate{}, nil
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var (
		updates []pm.PackageUpdate
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 4)
	)

	for _, crate := range crates {
		wg.Add(1)
		go func(cr pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest := fetchCratesIOLatestVersion(ctx, client, cr.Id)
			if latest != "" && latest != cr.Version {
				cr.AvailableVersion = latest
				mu.Lock()
				updates = append(updates, cr)
				mu.Unlock()
			}
		}(crate)
	}
	wg.Wait()

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})
	return updates, nil
}

func (c *CargoManager) UpgradePackage(ctx context.Context, id string) error {
	exe, err := cargoPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "install", id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cargo install failed for %s: %w", id, err)
	}
	return nil
}

func (c *CargoManager) UpgradeAll(ctx context.Context) error {
	updates, err := c.GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	for _, cr := range updates {
		if err := c.UpgradePackage(ctx, cr.Id); err != nil {
			return err
		}
	}
	return nil
}
