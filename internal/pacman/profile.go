package pacman

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
	_ pm.Lister    = (*PacmanManager)(nil)
	_ pm.Installer = (*PacmanManager)(nil)
)

// ListInstalled runs `pacman -Q` and parses installed Arch Linux packages.
func (p *PacmanManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := p.Path()
	if err != nil {
		return nil, err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"-Q"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("pacman -Q failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("pacman -Q failed: %w", err)
	}

	return parsePacmanInstalled(stdout.String()), nil
}

func parsePacmanInstalled(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "warning:") || strings.HasPrefix(line, "error:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			id := fields[0]
			ver := fields[1]
			if pm.ValidatePacmanID(id) == nil {
				pkgs = append(pkgs, pm.InstalledPackage{
					Name:        id,
					ID:          id,
					Version:     ver,
					ManagerName: "pacman",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a pacman package via `pacman -S --noconfirm <id>`.
func (p *PacmanManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
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

	runExe, runArgs := pm.WrapSudo(exe, []string{"-S", "--noconfirm", id})
	if err := runner.Run(ctx, runExe, runArgs, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pacman install failed for %s: %w", id, err)
	}
	return nil
}
