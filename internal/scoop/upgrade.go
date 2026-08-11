package scoop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UpgradePackage upgrades a specific package by its ID using scoop update.
func UpgradePackage(ctx context.Context, id string) error {
	exe, err := scoopPath()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, exe, "update", id)

	var stderrBuf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("scoop update failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("scoop update failed: %w", err)
	}
	return nil
}

// UpgradeAll upgrades all installed scoop packages using scoop update *.
func UpgradeAll(ctx context.Context) error {
	exe, err := scoopPath()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, exe, "update", "*")

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
