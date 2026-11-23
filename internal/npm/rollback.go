package npm

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*NpmManager)(nil)

// RollbackPackage installs a specific target version of a global npm package.
func (n *NpmManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := n.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := n.runner
	exe, err := npmPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "npm"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := n.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := n.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"install", "-g", "--", id + "@" + targetVersion}, n.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("npm rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
