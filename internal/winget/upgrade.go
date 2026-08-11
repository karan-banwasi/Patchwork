package winget

import (
	"context"
	"os"
	"os/exec"
)

// UpgradePackage upgrades a specific package by its ID.
func UpgradePackage(ctx context.Context, id string) error {
	exe, err := wingetPath()
	if err != nil {
		return err
	}
	// We use the --silent or --quiet flag depending on the manager
	// Winget supports --silent and --accept-source-agreements for smoother CLI flows
	cmd := exec.CommandContext(ctx, exe, "upgrade", "--id", id, "--silent", "--accept-source-agreements", "--accept-package-agreements")
	
	// Stream the output directly to the CLI so the user sees progress
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}

// UpgradeAll upgrades all available packages.
func UpgradeAll(ctx context.Context) error {
	exe, err := wingetPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "upgrade", "--all", "--silent", "--accept-source-agreements", "--accept-package-agreements")
	
	// Stream the output directly to the CLI
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}
