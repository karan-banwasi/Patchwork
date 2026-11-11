package pnpm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*PnpmManager)(nil)
	_ pm.Locator        = (*PnpmManager)(nil)
	_ pm.Cleaner        = (*PnpmManager)(nil)
	_ pm.BulkUpgrader   = (*PnpmManager)(nil)
	_ pm.IOConfigurable = (*PnpmManager)(nil)
)

// PnpmManager implements the pm.Manager interface for pnpm global packages.
type PnpmManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewPnpmManager() *PnpmManager {
	return &PnpmManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (p *PnpmManager) Name() string {
	return "pnpm"
}

func (p *PnpmManager) Aliases() []string {
	return []string{"pnpm-global"}
}

func (p *PnpmManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	p.stdin = stdin
	if stdout != nil {
		p.stdout = stdout
	} else {
		p.stdout = io.Discard
	}
	if stderr != nil {
		p.stderr = stderr
	} else {
		p.stderr = io.Discard
	}
}

func (p *PnpmManager) ValidateID(id string) error {
	return pm.ValidateNpmID(id)
}

func (p *PnpmManager) Path() (string, error) {
	if p.exePath != "" {
		return p.exePath, nil
	}
	for _, name := range []string{"pnpm.cmd", "pnpm.exe", "pnpm.bat", "pnpm"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("pnpm executable not found")
}

func (p *PnpmManager) Version(ctx context.Context) (string, error) {
	exe, err := p.Path()
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

func (p *PnpmManager) IsAvailable(ctx context.Context) bool {
	_, err := p.Path()
	return err == nil
}

func (p *PnpmManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := p.Path()
	if err != nil {
		return nil
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Pruning pnpm global store ---")
	return runner.Run(ctx, exe, []string{"store", "prune"}, p.stdin, out, errOut)
}

func (p *PnpmManager) UpgradePackage(ctx context.Context, id string) error {
	return p.UpgradePackages(ctx, []string{id})
}

func (p *PnpmManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := p.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := p.Path()
	if err != nil {
		return err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := p.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := p.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"update", "-g"}, ids...)
	if err := runner.Run(ctx, exe, args, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pnpm update failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (p *PnpmManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := p.Path()
	if err != nil {
		return nil, err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"outdated", "-g", "--format", "json"}, nil, &stdout, &stderr)
	// pnpm outdated returns exit code 1 if packages are outdated
	output := stdout.String()
	if err != nil && strings.TrimSpace(output) == "" {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("pnpm outdated failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("pnpm outdated failed: %w", err)
	}

	return parsePnpmOutdatedJSON(output)
}

type pnpmOutdatedEntry struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Wanted  string `json:"wanted"`
}

func parsePnpmOutdatedJSON(output string) ([]pm.PackageUpdate, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "{}" || output == "[]" {
		return []pm.PackageUpdate{}, nil
	}

	var data map[string]pnpmOutdatedEntry
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, fmt.Errorf("failed to parse pnpm outdated JSON: %w", err)
	}

	var updates []pm.PackageUpdate
	for name, entry := range data {
		if pm.ValidateNpmID(name) != nil {
			continue
		}
		curVer := strings.TrimSpace(entry.Current)
		availVer := strings.TrimSpace(entry.Latest)
		if curVer != "" && availVer != "" && version.IsUpgrade("pnpm", curVer, availVer) {
			updates = append(updates, pm.PackageUpdate{
				Name:             name,
				Id:               name,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "npm",
				ManagerName:      "pnpm",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
