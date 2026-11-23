package gem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*GemManager)(nil)
	_ pm.Locator        = (*GemManager)(nil)
	_ pm.Cleaner        = (*GemManager)(nil)
	_ pm.BulkUpgrader   = (*GemManager)(nil)
	_ pm.IOConfigurable = (*GemManager)(nil)
)

// GemManager implements the pm.Manager interface for RubyGems.
type GemManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewGemManager() *GemManager {
	return &GemManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (g *GemManager) Name() string {
	return "gem"
}

func (g *GemManager) Aliases() []string {
	return []string{"rubygems", "ruby"}
}

func (g *GemManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	g.stdin = stdin
	if stdout != nil {
		g.stdout = stdout
	} else {
		g.stdout = io.Discard
	}
	if stderr != nil {
		g.stderr = stderr
	} else {
		g.stderr = io.Discard
	}
}

func (g *GemManager) ValidateID(id string) error {
	return pm.ValidateGemID(id)
}

func (g *GemManager) Path() (string, error) {
	if g.exePath != "" {
		return g.exePath, nil
	}
	for _, name := range []string{"gem.cmd", "gem.bat", "gem"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("gem executable not found")
}

func (g *GemManager) Version(ctx context.Context) (string, error) {
	exe, err := g.Path()
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

func (g *GemManager) IsAvailable(ctx context.Context) bool {
	_, err := g.Path()
	return err == nil
}

func (g *GemManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := g.Path()
	if err != nil {
		return nil
	}
	runner := g.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up old gem versions ---")
	return runner.Run(ctx, exe, []string{"cleanup"}, g.stdin, out, errOut)
}

func (g *GemManager) UpgradePackage(ctx context.Context, id string) error {
	return g.UpgradePackages(ctx, []string{id})
}

func (g *GemManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := g.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := g.Path()
	if err != nil {
		return err
	}
	runner := g.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := g.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := g.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"update"}, ids...)
	if err := runner.Run(ctx, exe, args, g.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("gem update failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (g *GemManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := g.Path()
	if err != nil {
		return nil, err
	}
	runner := g.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"outdated"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("gem outdated failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("gem outdated failed: %w", err)
	}

	return parseGemOutdated(stdout.String())
}

var gemLineRegex = regexp.MustCompile(`^([a-zA-Z0-9._\-]+)\s+\((.+?)\s+<\s+(.+?)\)`)

func parseGemOutdated(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		m := gemLineRegex.FindStringSubmatch(line)
		if len(m) == 4 {
			id := m[1]
			curVer := strings.TrimSpace(m[2])
			availVer := strings.TrimSpace(m[3])

			if pm.ValidateGemID(id) != nil {
				continue
			}

			if version.IsUpgrade("gem", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             id,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           "rubygems",
					ManagerName:      "gem",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
