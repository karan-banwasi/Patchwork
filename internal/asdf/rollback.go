package asdf

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*AsdfManager)(nil)

// RollbackPackage installs and activates a specific target version of an asdf tool.
func (a *AsdfManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := a.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := a.runner
	exe, err := a.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "asdf"
		} else {
			return err
		}
	}
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

	if err := runner.Run(ctx, exe, []string{"install", id, targetVersion}, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("asdf install failed for %s %s: %w", id, targetVersion, err)
	}
	if err := runner.Run(ctx, exe, []string{"set", "-u", id, targetVersion}, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("asdf set -u failed for %s %s: %w", id, targetVersion, err)
	}
	return nil
}
