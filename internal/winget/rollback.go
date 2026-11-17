package winget

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*WingetManager)(nil)

func buildRollbackArgs(id, targetVersion string, acceptAgreements bool) []string {
	args := []string{"install", "--id", id, "--version", targetVersion, "--exact", "--force"}
	if acceptAgreements {
		if !shouldOmitSilent(id) {
			args = append(args, "--silent")
		}
		args = append(args, "--accept-source-agreements", "--accept-package-agreements")
	}
	return args
}

// RollbackPackage installs a specific target version of a winget package.
func (w *WingetManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := w.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := w.runner
	exe, err := wingetPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "winget"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := w.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := w.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := buildRollbackArgs(id, targetVersion, w.AcceptAgreements)
	cleanID := strings.ToLower(strings.TrimSpace(id))

	if pm.IsExecRunner(runner) {
		if strings.HasPrefix(cleanID, "microsoft.powershell") {
			fmt.Fprintln(stdout, "Note: Rolling back PowerShell requires Windows Installer to close running PowerShell sessions.")
			return runIsolatedUpgrade(ctx, exe, id, args)
		}
		if strings.HasPrefix(cleanID, "microsoft.windowsterminal") {
			fmt.Fprintln(stdout, "Note: Rolling back Windows Terminal will close or restart your terminal session.")
			return runIsolatedUpgrade(ctx, exe, id, args)
		}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, w.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget rollback failed for %s@%s: %w (%s)", id, targetVersion, err, stderrStr)
		}
		return fmt.Errorf("winget rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
