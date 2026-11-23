package uv

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*UvManager)(nil)

// RollbackPackage installs a specific target version of a uv tool.
func (u *UvManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := u.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := u.runner
	exe, err := uvPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "uv"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := u.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := u.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"tool", "install", id + "==" + targetVersion, "--force"}, u.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("uv rollback failed for %s==%s: %w", id, targetVersion, err)
	}
	return nil
}
