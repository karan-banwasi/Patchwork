package rustup

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
	_ pm.Manager        = (*RustupManager)(nil)
	_ pm.Locator        = (*RustupManager)(nil)
	_ pm.BulkUpgrader   = (*RustupManager)(nil)
	_ pm.IOConfigurable = (*RustupManager)(nil)
)

// RustupManager implements the pm.Manager interface for Rust toolchains managed by rustup.
type RustupManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewRustupManager() *RustupManager {
	return &RustupManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (r *RustupManager) Name() string {
	return "rustup"
}

func (r *RustupManager) Aliases() []string {
	return []string{"rust-toolchain"}
}

func (r *RustupManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	r.stdin = stdin
	if stdout != nil {
		r.stdout = stdout
	} else {
		r.stdout = io.Discard
	}
	if stderr != nil {
		r.stderr = stderr
	} else {
		r.stderr = io.Discard
	}
}

func (r *RustupManager) ValidateID(id string) error {
	return pm.ValidateRustupID(id)
}

func (r *RustupManager) Path() (string, error) {
	if r.exePath != "" {
		return r.exePath, nil
	}
	for _, name := range []string{"rustup.exe", "rustup"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("rustup executable not found")
}

func (r *RustupManager) Version(ctx context.Context) (string, error) {
	exe, err := r.Path()
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

func (r *RustupManager) IsAvailable(ctx context.Context) bool {
	_, err := r.Path()
	return err == nil
}

func (r *RustupManager) UpgradePackage(ctx context.Context, id string) error {
	return r.UpgradePackages(ctx, []string{id})
}

func (r *RustupManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := r.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := r.Path()
	if err != nil {
		return err
	}
	runner := r.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := r.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := r.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"update"}, ids...)
	if err := runner.Run(ctx, exe, args, r.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("rustup update failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (r *RustupManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := r.Path()
	if err != nil {
		return nil, err
	}
	runner := r.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"check"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("rustup check failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("rustup check failed: %w", err)
	}

	return parseRustupCheck(stdout.String())
}

var rustupLineRegex = regexp.MustCompile(`^([^\s]+)\s+-\s+Update available\s+:\s+([^\s]+)\s+->\s+([^\s]+)`)

func parseRustupCheck(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.Contains(line, "Up to date") {
			continue
		}

		m := rustupLineRegex.FindStringSubmatch(line)
		if len(m) == 4 {
			id := m[1]
			curVer := m[2]
			availVer := m[3]

			if pm.ValidateRustupID(id) != nil {
				continue
			}

			if version.IsUpgrade("rustup", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             id,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           "rustup",
					ManagerName:      "rustup",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
