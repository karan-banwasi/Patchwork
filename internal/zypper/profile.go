package zypper

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
	_ pm.Lister    = (*ZypperManager)(nil)
	_ pm.Installer = (*ZypperManager)(nil)
)

// ListInstalled runs `zypper --terse packages --installed-only` and parses installed packages.
func (z *ZypperManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := z.Path()
	if err != nil {
		return nil, err
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"--terse", "packages", "--installed-only"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("zypper packages failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("zypper packages failed: %w", err)
	}

	return parseZypperInstalled(stdout.String()), nil
}

func parseZypperInstalled(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	seen := make(map[string]bool)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if !strings.HasPrefix(parts[0], "i") {
			continue
		}
		id := parts[2]
		ver := parts[3]
		if pm.ValidateZypperID(id) == nil && !seen[id] {
			seen[id] = true
			pkgs = append(pkgs, pm.InstalledPackage{
				Name:        id,
				ID:          id,
				Version:     ver,
				ManagerName: "zypper",
			})
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a zypper package (optionally specifying version via `id=version`).
func (z *ZypperManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := z.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := z.Path()
	if err != nil {
		return err
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := z.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := z.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "=" + version
	}
	runExe, runArgs := pm.WrapSudo(exe, []string{"--non-interactive", "install", target})
	if err := runner.Run(ctx, runExe, runArgs, z.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("zypper install failed for %s: %w", id, err)
	}
	return nil
}
