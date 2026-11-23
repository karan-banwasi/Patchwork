package choco

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*ChocoManager)(nil)

// RollbackPackage installs a specific target version of a Chocolatey package with downgrade support.
func (c *ChocoManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := c.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := []string{"install", id, "--version", targetVersion, "--allow-downgrade", "-y", "--no-progress"}
	runner := c.runner
	prodRunner := IsProductionRunner(runner)

	if prodRunner && !isElevated() {
		exe, err := trustedChocoPath()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Chocolatey requires Administrator privileges to roll back %s. Requesting UAC elevation...\n", id)
		return runElevated(ctx, exe, args...)
	}

	if runner == nil {
		runner = pm.ExecRunner{}
	}
	exe, err := chocoPath()
	if err != nil {
		if !prodRunner {
			exe = "choco"
		} else {
			return err
		}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, c.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("choco rollback failed for %s@%s: %w (%s)", id, targetVersion, err, stderrStr)
		}
		return fmt.Errorf("choco rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
