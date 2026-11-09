package cargo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// CargoManager implements the pm.Manager interface for Rust global binaries installed via `cargo install`.
type CargoManager struct {
	runner pm.Runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func NewCargoManager() *CargoManager {
	return &CargoManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (c *CargoManager) Name() string {
	return "cargo"
}

func (c *CargoManager) Aliases() []string {
	return []string{"rust"}
}

func (c *CargoManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	c.stdin = stdin
	if stdout != nil {
		c.stdout = stdout
	} else {
		c.stdout = io.Discard
	}
	if stderr != nil {
		c.stderr = stderr
	} else {
		c.stderr = io.Discard
	}
}

func (c *CargoManager) ValidateID(id string) error {
	return pm.ValidateCargoID(id)
}

func (c *CargoManager) Path() (string, error) {
	return cargoPath()
}

func (c *CargoManager) Version(ctx context.Context) (string, error) {
	exe, err := cargoPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "cargo "), nil
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
		if len(m) == 3 && pm.ValidateCargoID(m[1]) == nil {
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

func fetchCratesIOLatestVersion(ctx context.Context, client *http.Client, crateName string) (string, error) {
	url := fmt.Sprintf("https://crates.io/api/v1/crates/%s", crateName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Patchwork-CLI (https://github.com/karan-banwasi/Patchwork)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var data cratesIOResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	if data.Crate.MaxStableVersion != "" {
		return strings.TrimSpace(data.Crate.MaxStableVersion), nil
	}
	return strings.TrimSpace(data.Crate.NewestVersion), nil
}

// GetAvailableUpdatesWithWarnings queries crates.io and collects partial failure warnings.
func (c *CargoManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	exe, err := cargoPath()
	if err != nil {
		return nil, nil, err
	}

	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"install", "--list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("failed to list installed cargo packages: %w: %s", err, errMsg)
		}
		return nil, nil, fmt.Errorf("failed to list installed cargo packages: %w", err)
	}

	crates := parseCargoInstallList(stdout.String())
	if len(crates) == 0 {
		return []pm.PackageUpdate{}, nil, nil
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var (
		updates    []pm.PackageUpdate
		lookupErrs []error
		mu         sync.Mutex
		wg         sync.WaitGroup
		sem        = make(chan struct{}, 4)
	)

	for _, crate := range crates {
		wg.Add(1)
		go func(cr pm.PackageUpdate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			latest, err := fetchCratesIOLatestVersion(ctx, client, cr.Id)
			if err != nil {
				mu.Lock()
				lookupErrs = append(lookupErrs, fmt.Errorf("%s: %w", cr.Id, err))
				mu.Unlock()
				return
			}
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

	if len(lookupErrs) > 0 {
		if len(lookupErrs) == len(crates) {
			return nil, nil, fmt.Errorf("failed to check any crates (0 of %d checked): %v", len(crates), lookupErrs[0])
		}
		warning := fmt.Errorf("[%s] %d of %d crates could not be checked; network error: %v", c.Name(), len(lookupErrs), len(crates), lookupErrs[0])
		return updates, []error{warning}, nil
	}

	return updates, nil, nil
}

func (c *CargoManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	updates, _, err := c.GetAvailableUpdatesWithWarnings(ctx)
	return updates, err
}

func (c *CargoManager) UpgradePackage(ctx context.Context, id string) error {
	if err := c.ValidateID(id); err != nil {
		return err
	}
	exe, err := cargoPath()
	if err != nil {
		return err
	}
	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if err := runner.Run(ctx, exe, []string{"install", id}, c.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("cargo install failed for %s: %w", id, err)
	}
	return nil
}
