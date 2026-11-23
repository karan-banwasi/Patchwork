package mise

import (
	"context"
	"fmt"
	"io"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*MiseManager)(nil)

// RollbackPackage installs and activates a specific target version of a mise tool globally.
func (m *MiseManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := m.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := m.runner
	exe, err := m.Path()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "mise"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := m.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := m.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	spec := id + "@" + targetVersion
	if err := runner.Run(ctx, exe, []string{"install", spec}, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mise install failed for %s: %w", spec, err)
	}
	if err := runner.Run(ctx, exe, []string{"use", "-g", spec}, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mise use -g failed for %s: %w", spec, err)
	}
	return nil
}
