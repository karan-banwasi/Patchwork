package bun

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*BunManager)(nil)
	_ pm.Installer = (*BunManager)(nil)
)

// ListInstalled runs `bun pm ls -g` and parses globally installed bun packages.
func (b *BunManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := b.Path()
	if err != nil {
		return nil, err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"pm", "ls", "-g"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("bun pm ls -g failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("bun pm ls -g failed: %w", err)
	}

	parsed, err := parseBunGlobalList(stdout.String())
	if err != nil {
		return nil, err
	}
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, p := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        p.Name,
			ID:          p.Id,
			Version:     p.Version,
			ManagerName: "bun",
		})
	}
	return pkgs, nil
}

// InstallPackage installs a global bun package (optionally specifying version via `id@version`).
func (b *BunManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := b.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := b.Path()
	if err != nil {
		return err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := b.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := b.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "@" + version
	}
	if err := runner.Run(ctx, exe, []string{"add", "-g", target}, b.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("bun install failed for %s: %w", id, err)
	}
	return nil
}
