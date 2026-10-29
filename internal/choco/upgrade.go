package choco

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// UpgradePackage upgrades a specific package by its ID using choco upgrade -y.
func UpgradePackage(ctx context.Context, id string) error {
	exe, err := chocoPath()
	if err != nil {
		return err
	}

	if !isElevated() {
		fmt.Printf("Chocolatey requires Administrator privileges to upgrade %s. Requesting UAC elevation...\n", id)
		return runElevated(exe, "upgrade", id, "-y", "--no-progress")
	}

	cmd := exec.CommandContext(ctx, exe, "upgrade", id, "-y", "--no-progress")

	var stderrBuf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if strings.Contains(stderrStr, "Access to the path") || strings.Contains(stderrStr, "denied") || strings.Contains(stderrStr, "elevated command shell") {
			return fmt.Errorf("chocolatey requires Administrator privileges to upgrade packages. Please rerun Patchwork in an elevated shell (Run as Administrator)")
		}
		if stderrStr != "" {
			return fmt.Errorf("choco upgrade failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("choco upgrade failed: %w", err)
	}
	return nil
}

// UpgradeAll upgrades all installed chocolatey packages using choco upgrade all -y.
func UpgradeAll(ctx context.Context) error {
	exe, err := chocoPath()
	if err != nil {
		return err
	}

	if !isElevated() {
		fmt.Println("Chocolatey requires Administrator privileges to upgrade packages. Requesting UAC elevation...")
		return runElevated(exe, "upgrade", "all", "-y", "--no-progress")
	}

	cmd := exec.CommandContext(ctx, exe, "upgrade", "all", "-y", "--no-progress")

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
