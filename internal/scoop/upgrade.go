package scoop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// UpgradePackage upgrades a specific package by its ID using scoop update.
func (s *ScoopManager) UpgradePackage(ctx context.Context, id string) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}

	exe, err := scoopPath()
	if err != nil {
		return err
	}

	stdout := s.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := s.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"update", id}, s.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("scoop update failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("scoop update failed: %w", err)
	}
	return nil
}

// UpgradePackage is a package-level helper for convenience.
func UpgradePackage(ctx context.Context, id string) error {
	return NewScoopManager().UpgradePackage(ctx, id)
}
