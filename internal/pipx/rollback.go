package pipx

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*PipxManager)(nil)

// RollbackPackage installs a specific target version of a pipx application.
func (p *PipxManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
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
			exe = "pipx"
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

	if err := runner.Run(ctx, exe, []string{"install", id + "==" + targetVersion, "--force"}, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pipx rollback failed for %s==%s: %w", id, targetVersion, err)
	}
	return nil
}
