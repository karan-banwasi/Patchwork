package winget

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// shouldOmitSilent returns true if the package is known to cause abrupt process
// termination or issues when upgraded in silent mode (such as the host shell or terminal).
func shouldOmitSilent(id string) bool {
	lower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasPrefix(lower, "microsoft.powershell") ||
		strings.HasPrefix(lower, "microsoft.windowsterminal")
}

// buildUpgradeArgs constructs the winget command-line arguments for upgrading a package.
func buildUpgradeArgs(id string) []string {
	args := []string{"upgrade", "--id", id, "--exact"}
	if !shouldOmitSilent(id) {
		args = append(args, "--silent")
	}
	args = append(args, "--accept-source-agreements", "--accept-package-agreements")
	return args
}

// UpgradePackage upgrades a specific package by its ID.
func UpgradePackage(ctx context.Context, id string) error {
	exe, err := wingetPath()
	if err != nil {
		return err
	}

	cleanId := strings.ToLower(strings.TrimSpace(id))
	if strings.HasPrefix(cleanId, "microsoft.powershell") {
		fmt.Println("Note: Upgrading PowerShell from inside PowerShell will restart your shell session once the install completes.")
	} else if strings.HasPrefix(cleanId, "microsoft.windowsterminal") {
		fmt.Println("Note: Upgrading Windows Terminal will close or restart your terminal session once the install completes.")
	}

	// We use the --silent flag depending on whether the package is an active shell/terminal.
	// Winget supports --exact, --silent, and agreements flags for deterministic CLI flows.
	args := buildUpgradeArgs(id)
	cmd := exec.CommandContext(ctx, exe, args...)
	
	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderrBuf
	cmd.Stdin = os.Stdin
	
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget upgrade failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("winget upgrade failed: %w", err)
	}
	return nil
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
