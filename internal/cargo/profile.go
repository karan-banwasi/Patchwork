package cargo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*CargoManager)(nil)
	_ pm.Installer = (*CargoManager)(nil)
)

func isMockRunner(r pm.Runner) bool {
	if r == nil {
		return false
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return !isExec && !isExecPtr
}

func (c *CargoManager) resolveExe() (string, error) {
	exe, err := cargoPath()
	if err != nil {
		if isMockRunner(c.runner) {
			return "cargo", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `cargo install --list` and parses installed Rust crates.
func (c *CargoManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := c.resolveExe()
	if err != nil {
		return nil, err
	}
	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"install", "--list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("cargo install --list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("cargo install --list failed: %w", err)
	}

	parsed := parseCargoInstallList(stdout.String())
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, cr := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        cr.Name,
			ID:          cr.Id,
			Version:     cr.Version,
			ManagerName: "cargo",
		})
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a Rust crate via `cargo install <id>` (with optional `--version`).
func (c *CargoManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := c.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := c.resolveExe()
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

	args := []string{"install", id}
	if version != "" {
		args = append(args, "--version", version)
	}
	if err := runner.Run(ctx, exe, args, c.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("cargo install failed for %s: %w", id, err)
	}
	return nil
}
