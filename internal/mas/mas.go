package mas

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*MasManager)(nil)
	_ pm.Locator        = (*MasManager)(nil)
	_ pm.BulkUpgrader   = (*MasManager)(nil)
	_ pm.IOConfigurable = (*MasManager)(nil)
)

// MasManager implements the pm.Manager interface for Mac App Store (mas-cli) on macOS.
type MasManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewMasManager() *MasManager {
	return &MasManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (m *MasManager) Name() string {
	return "mas"
}

func (m *MasManager) Aliases() []string {
	return []string{"macappstore", "appstore"}
}

func (m *MasManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	m.stdin = stdin
	if stdout != nil {
		m.stdout = stdout
	} else {
		m.stdout = io.Discard
	}
	if stderr != nil {
		m.stderr = stderr
	} else {
		m.stderr = io.Discard
	}
}

func (m *MasManager) ValidateID(id string) error {
	return pm.ValidateMasID(id)
}

func (m *MasManager) Path() (string, error) {
	if m.exePath != "" {
		return m.exePath, nil
	}
	return masPath()
}

func (m *MasManager) Version(ctx context.Context) (string, error) {
	exe, err := m.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

func (m *MasManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := m.Path()
	return err == nil
}

func masPath() (string, error) {
	for _, name := range []string{"mas"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/opt/homebrew/bin/mas", "/usr/local/bin/mas"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("mas executable not found")
}

func (m *MasManager) UpgradePackage(ctx context.Context, id string) error {
	return m.UpgradePackages(ctx, []string{id})
}

func (m *MasManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := m.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := m.Path()
	if err != nil {
		return err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := m.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := m.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"upgrade"}, ids...)
	if err := runner.Run(ctx, exe, args, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mas upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (m *MasManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := m.Path()
	if err != nil {
		return nil, err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"outdated"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("mas outdated failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("mas outdated failed: %w", err)
	}

	return parseMasOutdated(stdout.String())
}

var masLineRegex = regexp.MustCompile(`^([0-9]+)\s+(.+?)\s+\((.+?)\s+->\s+(.+?)\)$`)

func parseMasOutdated(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Warning:") {
			continue
		}

		m := masLineRegex.FindStringSubmatch(line)
		if len(m) == 5 {
			id := m[1]
			name := strings.TrimSpace(m[2])
			curVer := strings.TrimSpace(m[3])
			availVer := strings.TrimSpace(m[4])

			if pm.ValidateMasID(id) != nil {
				continue
			}

			if version.IsUpgrade("mas", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             name,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           "mas",
					ManagerName:      "mas",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
