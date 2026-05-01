package winget

import (
	"os"
	"os/exec"
)

// UpgradePackage upgrades a specific package by its ID.
func UpgradePackage(id string) error {
	// We use the --silent or --quiet flag depending on the manager
	// Winget supports --silent and --accept-source-agreements for smoother CLI flows
	cmd := exec.Command("winget", "upgrade", "--id", id, "--silent", "--accept-source-agreements", "--accept-package-agreements")
	
	// Stream the output directly to the CLI so the user sees progress
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}

// UpgradeAll upgrades all available packages.
func UpgradeAll() error {
	cmd := exec.Command("winget", "upgrade", "--all", "--silent", "--accept-source-agreements", "--accept-package-agreements")
	
	// Stream the output directly to the CLI
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}
