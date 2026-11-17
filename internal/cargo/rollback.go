package cargo

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*CargoManager)(nil)

// RollbackPackage installs a specific target version of a Cargo crate.
func (c *CargoManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := c.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := c.runner
	exe, err := cargoPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "cargo"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"install", id, "--version", targetVersion, "--force"}, c.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("cargo rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
