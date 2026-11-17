package vscode

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*VSCodeManager)(nil)

// RollbackPackage installs a specific target version of a VS Code extension.
func (v *VSCodeManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := v.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := v.runner
	exe, err := v.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "code"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := v.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := v.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"--install-extension", id + "@" + targetVersion, "--force"}, v.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("vscode rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
