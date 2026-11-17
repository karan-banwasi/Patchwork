package pnpm

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*PnpmManager)(nil)

// RollbackPackage installs a specific target version of a global pnpm package.
func (p *PnpmManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := p.runner
	exe, err := p.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "pnpm"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := p.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := p.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"add", "-g", id + "@" + targetVersion}, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pnpm rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
