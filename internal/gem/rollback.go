package gem

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*GemManager)(nil)

// RollbackPackage installs a specific target version of a RubyGem.
func (g *GemManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := g.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := g.runner
	exe, err := g.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "gem"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := g.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := g.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"install", id, "-v", targetVersion}, g.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("gem rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
