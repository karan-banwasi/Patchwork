package dotnet

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
	_ pm.Lister    = (*DotnetManager)(nil)
	_ pm.Installer = (*DotnetManager)(nil)
)

func (d *DotnetManager) resolveExe() (string, error) {
	exe, err := dotnetPath()
	if err != nil {
		if !pm.IsExecRunner(d.runner) {
			return "dotnet", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `dotnet tool list -g` and parses globally installed .NET tools.
func (d *DotnetManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := d.resolveExe()
	if err != nil {
		return nil, err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"tool", "list", "-g"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if isNoSDKError(errMsg) || isNoSDKError(stdout.String()) {
			return []pm.InstalledPackage{}, nil
		}
		if errMsg != "" {
			return nil, fmt.Errorf("dotnet tool list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("dotnet tool list failed: %w", err)
	}

	parsed := parseDotnetToolList(stdout.String())
	pkgs := make([]pm.InstalledPackage, 0, len(parsed))
	for _, t := range parsed {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        t.Name,
			ID:          t.Id,
			Version:     t.Version,
			ManagerName: "dotnet",
		})
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a global .NET tool via `dotnet tool install -g <id>`.
func (d *DotnetManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := d.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := d.resolveExe()
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

	args := []string{"tool", "install", "-g", id}
	if version != "" {
		args = append(args, "--version", version)
	}
	if err := runner.Run(ctx, exe, args, d.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("dotnet tool install failed for %s: %w", id, err)
	}
	return nil
}
