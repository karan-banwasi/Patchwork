package brew

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// BrewManager implements the pm.Manager interface for Homebrew formulas and casks.
type BrewManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewBrewManager() *BrewManager {
	return &BrewManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (b *BrewManager) Name() string {
	return "brew"
}

func (b *BrewManager) Aliases() []string {
	return []string{"homebrew"}
}

func (b *BrewManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	b.stdin = stdin
	if stdout != nil {
		b.stdout = stdout
	} else {
		b.stdout = io.Discard
	}
	if stderr != nil {
		b.stderr = stderr
	} else {
		b.stderr = io.Discard
	}
}

func (b *BrewManager) ValidateID(id string) error {
	return pm.ValidateBrewID(id)
}

func (b *BrewManager) Path() (string, error) {
	if b.exePath != "" {
		return b.exePath, nil
	}
	return brewPath()
}

func (b *BrewManager) Version(ctx context.Context) (string, error) {
	exe, err := b.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

func (b *BrewManager) IsAvailable(ctx context.Context) bool {
	_, err := b.Path()
	return err == nil
}

func (b *BrewManager) Refresh(ctx context.Context) error {
	exe, err := b.Path()
	if err != nil {
		return err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"update"}, nil, b.stdout, b.stderr)
}

func (b *BrewManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := b.Path()
	if err != nil {
		return nil
	}
	fmt.Fprintln(out, "--- Cleaning up Homebrew cache and old versions ---")
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"cleanup"}, nil, out, errOut)
}

func (b *BrewManager) UpgradePackage(ctx context.Context, id string) error {
	return b.UpgradePackages(ctx, []string{id})
}

func (b *BrewManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := b.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := b.Path()
	if err != nil {
		return err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := b.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := b.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"upgrade"}, ids...)
	if err := runner.Run(ctx, exe, args, b.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("brew upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (b *BrewManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := b.Path()
	if err != nil {
		return nil, err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"outdated", "--json=v2"}, nil, &stdout, &stderr); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("brew outdated failed: %w (%s)", err, stderrStr)
		}
		return nil, fmt.Errorf("brew outdated failed: %w", err)
	}

	return parseBrewOutdatedJSON(stdout.String())
}

func brewPath() (string, error) {
	if p, err := exec.LookPath("brew"); err == nil {
		return p, nil
	}
	standardPaths := []string{
		"/opt/homebrew/bin/brew",
		"/usr/local/bin/brew",
		"/home/linuxbrew/.linuxbrew/bin/brew",
	}
	if home := os.Getenv("HOME"); home != "" {
		standardPaths = append(standardPaths, filepath.Join(home, ".linuxbrew", "bin", "brew"))
	}
	for _, p := range standardPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("brew executable not found")
}

type brewOutdatedV2 struct {
	Formulae []brewFormulaOutdated `json:"formulae"`
	Casks    []brewCaskOutdated    `json:"casks"`
}

type brewFormulaOutdated struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
	PinnedVersion     *string  `json:"pinned_version"`
}

type brewCaskOutdated struct {
	Token             string      `json:"token"`
	Name              interface{} `json:"name"`
	InstalledVersions []string    `json:"installed_versions"`
	CurrentVersion    string      `json:"current_version"`
	Pinned            bool        `json:"pinned"`
	PinnedVersion     *string     `json:"pinned_version"`
}

func extractCaskName(rawName interface{}, token string) string {
	switch v := rawName.(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	case []string:
		if len(v) > 0 && strings.TrimSpace(v[0]) != "" {
			return strings.TrimSpace(v[0])
		}
	}
	return token
}

func parseBrewOutdatedJSON(output string) ([]pm.PackageUpdate, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "{}" {
		return []pm.PackageUpdate{}, nil
	}

	var root brewOutdatedV2
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		return nil, fmt.Errorf("failed to parse brew outdated JSON: %w", err)
	}

	var updates []pm.PackageUpdate

	for _, f := range root.Formulae {
		if f.Pinned {
			continue
		}
		if pm.ValidateBrewID(f.Name) != nil {
			continue
		}
		curVer := ""
		if len(f.InstalledVersions) > 0 {
			curVer = f.InstalledVersions[len(f.InstalledVersions)-1]
		}
		availVer := f.CurrentVersion
		if curVer != "" && availVer != "" && curVer != availVer {
			updates = append(updates, pm.PackageUpdate{
				Name:             f.Name,
				Id:               f.Name,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "homebrew",
				ManagerName:      "brew",
			})
		}
	}

	for _, c := range root.Casks {
		if c.Pinned {
			continue
		}
		id := c.Token
		if id == "" {
			id = extractCaskName(c.Name, "")
		}
		if id == "" || pm.ValidateBrewID(id) != nil {
			continue
		}
		displayName := extractCaskName(c.Name, id)
		curVer := ""
		if len(c.InstalledVersions) > 0 {
			curVer = c.InstalledVersions[len(c.InstalledVersions)-1]
		}
		availVer := c.CurrentVersion
		if curVer != "" && availVer != "" && curVer != availVer {
			updates = append(updates, pm.PackageUpdate{
				Name:             displayName,
				Id:               id,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "cask",
				ManagerName:      "brew",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
