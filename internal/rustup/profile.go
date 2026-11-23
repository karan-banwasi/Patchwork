package rustup

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
	_ pm.Lister    = (*RustupManager)(nil)
	_ pm.Installer = (*RustupManager)(nil)
)

// ListInstalled runs `rustup toolchain list` and parses installed Rust toolchains.
func (r *RustupManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := r.Path()
	if err != nil {
		return nil, err
	}
	runner := r.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"toolchain", "list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("rustup toolchain list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("rustup toolchain list failed: %w", err)
	}

	return parseRustupToolchainList(stdout.String()), nil
}

func parseRustupToolchainList(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "no installed toolchains") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		if pm.ValidateRustupID(id) == nil {
			pkgs = append(pkgs, pm.InstalledPackage{
				Name:        id,
				ID:          id,
				ManagerName: "rustup",
			})
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a Rustup toolchain via `rustup toolchain install <id>`.
func (r *RustupManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := r.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
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

	if err := runner.Run(ctx, exe, []string{"toolchain", "install", id}, r.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("rustup toolchain install failed for %s: %w", id, err)
	}
	return nil
}
