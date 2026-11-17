package choco

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
	_ pm.Lister    = (*ChocoManager)(nil)
	_ pm.Installer = (*ChocoManager)(nil)
)

func (c *ChocoManager) resolveExe() (string, error) {
	exe, err := chocoPath()
	if err != nil {
		if !isProductionRunner(c.runner) {
			return "choco", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `choco list --limit-output` to list locally installed Chocolatey packages.
func (c *ChocoManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := c.resolveExe()
	if err != nil {
		return nil, err
	}

	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--limit-output"}, nil, &stdout, &stderr); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("choco list failed: %w (%s)", err, stderrStr)
		}
		return nil, fmt.Errorf("choco list failed: %w", err)
	}

	return parseChocoListOutput(stdout.String()), nil
}

func parseChocoListOutput(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	clean := ansiEscape.ReplaceAllString(output, "")
	for _, rawLine := range strings.Split(clean, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Chocolatey v") || strings.Contains(line, "packages installed.") {
			continue
		}
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				id := strings.TrimSpace(parts[0])
				ver := strings.TrimSpace(parts[1])
				if id != "" && pm.ValidateChocoID(id) == nil {
					pkgs = append(pkgs, pm.InstalledPackage{
						Name:        id,
						ID:          id,
						Version:     ver,
						ManagerName: "choco",
					})
				}
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a Chocolatey package with optional version.
func (c *ChocoManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := c.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	args := []string{"install", id, "-y", "--no-progress"}
	if version != "" {
		args = append(args, "--version", version)
	}

	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if isProductionRunner(c.runner) && !isElevated() {
		exe, err := trustedChocoPath()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Chocolatey requires Administrator privileges to install %s. Requesting UAC elevation...\n", id)
		return runElevated(ctx, exe, args...)
	}

	exe, err := c.resolveExe()
	if err != nil {
		return err
	}
	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, c.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("choco install failed for %s: %w (%s)", id, err, stderrStr)
		}
		return fmt.Errorf("choco install failed for %s: %w", id, err)
	}
	return nil
}
