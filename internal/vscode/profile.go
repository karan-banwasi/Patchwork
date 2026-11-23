package vscode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*VSCodeManager)(nil)
	_ pm.Installer = (*VSCodeManager)(nil)
)

// ListInstalled runs `code --list-extensions --show-versions` and parses installed extensions.
func (v *VSCodeManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := v.Path()
	if err != nil {
		return nil, err
	}
	runner := v.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"--list-extensions", "--show-versions"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("code list extensions failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("code list extensions failed: %w", err)
	}

	parsed, err := parseVSCodeListExtensions(stdout.String())
	if err != nil {
		return nil, err
	}
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, ext := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        ext.Name,
			ID:          ext.Id,
			Version:     ext.Version,
			ManagerName: "vscode",
		})
	}
	return pkgs, nil
}

// InstallPackage installs a VS Code extension (optionally pinned via `id@version`).
func (v *VSCodeManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := v.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := v.Path()
	if err != nil {
		return err
	}
	runner := v.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := v.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := v.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "@" + version
	}
	if err := runner.Run(ctx, exe, []string{"--install-extension", target, "--force"}, v.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("code install extension failed for %s: %w", id, err)
	}
	return nil
}
