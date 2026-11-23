package uv

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
	_ pm.Lister    = (*UvManager)(nil)
	_ pm.Installer = (*UvManager)(nil)
)

func (u *UvManager) resolveExe() (string, error) {
	exe, err := uvPath()
	if err != nil {
		if !pm.IsExecRunner(u.runner) {
			return "uv", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `uv tool list` and parses installed Python tools.
func (u *UvManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := u.resolveExe()
	if err != nil {
		return nil, err
	}
	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"tool", "list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("uv tool list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("uv tool list failed: %w", err)
	}

	parsed := parseUvToolList(stdout.String())
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, t := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        t.Name,
			ID:          t.Id,
			Version:     t.Version,
			ManagerName: "uv",
		})
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a uv Python tool (optionally pinned via `id==version`).
func (u *UvManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := u.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := u.resolveExe()
	if err != nil {
		return err
	}
	runner := u.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := u.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := u.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "==" + version
	}
	if err := runner.Run(ctx, exe, []string{"tool", "install", target}, u.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("uv tool install failed for %s: %w", id, err)
	}
	return nil
}
