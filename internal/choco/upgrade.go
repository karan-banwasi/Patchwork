package choco

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// UpgradePackage upgrades a specific package by its ID using choco upgrade -y.
func (c *ChocoManager) UpgradePackage(ctx context.Context, id string) error {
	return c.UpgradePackages(ctx, []string{id})
}

// UpgradePackages upgrades one or more Chocolatey packages in a single invocation
// so that non-elevated runs trigger at most one UAC prompt.
func (c *ChocoManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := c.ValidateID(id); err != nil {
			return err
		}
	}

	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := make([]string, 0, len(ids)+3)
	args = append(args, "upgrade")
	args = append(args, ids...)
	args = append(args, "-y", "--no-progress")

	if !isElevated() {
		exe, err := trustedChocoPath()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Chocolatey requires Administrator privileges to upgrade %s. Requesting UAC elevation...\n", strings.Join(ids, ", "))
		return runElevated(ctx, exe, args...)
	}

	exe, err := chocoPath()
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
			return fmt.Errorf("choco upgrade failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("choco upgrade failed: %w", err)
	}
	return nil
}

// UpgradePackage is a package-level helper for convenience.
func UpgradePackage(ctx context.Context, id string) error {
	return NewChocoManager().UpgradePackage(ctx, id)
}
