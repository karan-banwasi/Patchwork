package dnf

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
	_ pm.Lister    = (*DnfManager)(nil)
	_ pm.Installer = (*DnfManager)(nil)
)

// ListInstalled runs `dnf list --installed --quiet` and parses installed RPM packages.
func (d *DnfManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := d.Path()
	if err != nil {
		return nil, err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--installed", "--quiet"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("dnf list --installed failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("dnf list --installed failed: %w", err)
	}

	return parseDnfInstalled(stdout.String()), nil
}

func parseDnfInstalled(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	seen := make(map[string]bool)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.EqualFold(line, "Installed Packages") {
			continue
		}
		m := dnfLineRegex.FindStringSubmatch(line)
		if len(m) == 5 {
			id := m[1]
			ver := m[3]
			if pm.ValidateDnfID(id) == nil && !seen[id] {
				seen[id] = true
				pkgs = append(pkgs, pm.InstalledPackage{
					Name:        id,
					ID:          id,
					Version:     ver,
					ManagerName: "dnf",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a DNF package (optionally specifying version via `id-version`).
func (d *DnfManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := d.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := d.Path()
	if err != nil {
		return err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := d.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := d.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "-" + version
	}
	runExe, runArgs := wrapSudo(exe, []string{"install", "-y", target})
	if err := runner.Run(ctx, runExe, runArgs, d.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("dnf install failed for %s: %w", id, err)
	}
	return nil
}
