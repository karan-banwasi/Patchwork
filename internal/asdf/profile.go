package asdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*AsdfManager)(nil)
	_ pm.Installer = (*AsdfManager)(nil)
)

// ListInstalled runs `asdf current` and parses configured/installed asdf tools.
func (a *AsdfManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := a.Path()
	if err != nil {
		return nil, err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"current"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("asdf current failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("asdf current failed: %w", err)
	}

	parsed, err := parseAsdfCurrent(stdout.String())
	if err != nil {
		return nil, err
	}
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, p := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        p.Name,
			ID:          p.Id,
			Version:     p.Version,
			ManagerName: "asdf",
		})
	}
	return pkgs, nil
}

// InstallPackage installs an asdf tool version via `asdf install <id> <version|latest>`.
func (a *AsdfManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := a.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := a.Path()
	if err != nil {
		return err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := a.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := a.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	ver := version
	if ver == "" {
		ver = "latest"
	}
	if err := runner.Run(ctx, exe, []string{"install", id, ver}, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("asdf install failed for %s: %w", id, err)
	}
	return nil
}
