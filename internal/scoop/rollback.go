package scoop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*ScoopManager)(nil)

// RollbackPackage switches a Scoop package to a specific previously installed version.
func (s *ScoopManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := s.runner
	exe, err := scoopPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "scoop"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := s.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := s.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"reset", id + "@" + targetVersion}, s.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("scoop rollback failed for %s@%s: %w (%s)", id, targetVersion, err, stderrStr)
		}
		return fmt.Errorf("scoop rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
