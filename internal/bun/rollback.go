package bun

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*BunManager)(nil)

// RollbackPackage installs a specific target version of a global bun package.
func (b *BunManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := b.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := b.runner
	exe, err := b.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "bun"
		} else {
			return err
		}
	}
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

	if err := runner.Run(ctx, exe, []string{"add", "-g", id + "@" + targetVersion}, b.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("bun rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
