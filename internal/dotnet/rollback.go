package dotnet

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*DotnetManager)(nil)

// RollbackPackage installs a specific target version of a global .NET tool.
func (d *DotnetManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := d.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := d.runner
	exe, err := dotnetPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "dotnet"
		} else {
			return err
		}
	}
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

	if err := runner.Run(ctx, exe, []string{"tool", "install", "-g", id, "--version", targetVersion}, d.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("dotnet rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
