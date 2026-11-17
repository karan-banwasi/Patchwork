package pipx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*PipxManager)(nil)
	_ pm.Installer = (*PipxManager)(nil)
)

// ListInstalled runs `pipx list --json` and parses installed Python applications.
func (p *PipxManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := p.Path()
	if err != nil {
		return nil, err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--json"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("pipx list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("pipx list failed: %w", err)
	}

	parsed, err := parsePipxListJSON(stdout.String())
	if err != nil {
		return nil, err
	}
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, item := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        item.Name,
			ID:          item.Id,
			Version:     item.Version,
			ManagerName: "pipx",
		})
	}
	return pkgs, nil
}

// InstallPackage installs a pipx Python application (optionally pinned via `id==version`).
func (p *PipxManager) InstallPackage(ctx context.Context, id, version string) error {
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

	target := id
	if version != "" {
		target = id + "==" + version
	}
	if err := runner.Run(ctx, exe, []string{"install", target}, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pipx install failed for %s: %w", id, err)
	}
	return nil
}
