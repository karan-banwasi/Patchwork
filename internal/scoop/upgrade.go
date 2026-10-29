package scoop

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("scoop update failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("scoop update failed: %w", err)
	}
	return nil
}

// UpgradeAll upgrades all outdated scoop packages.
func UpgradeAll(ctx context.Context) error {
	updates, err := GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	var hadErr bool
	for _, u := range updates {
		if err := UpgradePackage(ctx, u.Id); err != nil {
			hadErr = true
		}
	}
	if hadErr {
		return fmt.Errorf("one or more scoop package upgrades failed")
	}
	return nil
}
