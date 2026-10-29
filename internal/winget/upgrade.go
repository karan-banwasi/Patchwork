package winget

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// buildIsolatedUpgradeScript constructs the batch script used to run self-updating shell
// or terminal upgrades in a standalone console window so that closing PowerShell or
// Windows Terminal does not kill winget mid-install or swallow installer errors.
func buildIsolatedUpgradeScript(wingetExe string, id string, args []string, statusFile string) string {
	quotedArgs := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			quotedArgs[i] = fmt.Sprintf(`"%s"`, strings.ReplaceAll(a, `"`, `""`))
		} else {
			quotedArgs[i] = a
		}
	}
	cmdLine := fmt.Sprintf(`"%s" %s`, wingetExe, strings.Join(quotedArgs, " "))

	lines := []string{
		"@echo off",
		fmt.Sprintf("title Patchwork - Upgrading %s", id),
		"echo ===============================================================================",
		fmt.Sprintf("echo  Patchwork: Upgrading %s", id),
		"echo  Windows Installer will close running PowerShell/Terminal sessions during setup.",
		"echo  IMPORTANT: Do NOT open a new PowerShell window until this window completes,",
		"echo  or Windows Installer will fail with MSI error 1603.",
		"echo ===============================================================================",
		"echo.",
		cmdLine,
		`set "EXIT_CODE=%ERRORLEVEL%"`,
		fmt.Sprintf(`> "%s" echo %%EXIT_CODE%%`, statusFile),
		"echo.",
		`if NOT "%EXIT_CODE%"=="0" (`,
		"    echo ===============================================================================",
		fmt.Sprintf("    echo  [ERROR] winget upgrade for %s failed ^(exit code: %%EXIT_CODE%%^).", id),
		"    echo  If MSI failed with code 1603, another PowerShell session ^(such as an IDE",
		"    echo  terminal or extension^) was still running or reopened during the upgrade.",
		"    echo  Close all other PowerShell processes and retry.",
		"    echo ===============================================================================",
		"    pause",
		"    exit /b %EXIT_CODE%",
		")",
		"echo ===============================================================================",
		fmt.Sprintf("echo  [OK] Successfully upgraded %s!", id),
		"echo ===============================================================================",
		"timeout /t 3 >nul",
		"exit /b 0",
		"",
	}
	return strings.Join(lines, "\r\n")
}

// parseIsolatedUpgradeStatus interprets the exit code written to the status file
// by the isolated console script, ensuring non-zero exit codes and premature window
// closures are always surfaced as errors.
func parseIsolatedUpgradeStatus(id string, statusContent string, readErr error, runErr error) error {
	if readErr == nil {
		codeStr := strings.TrimSpace(statusContent)
		if codeStr != "" && codeStr != "0" {
			return fmt.Errorf("winget upgrade failed for %s (exit code %s)", id, codeStr)
		}
		if codeStr == "0" {
			return nil
		}
	}
	if runErr != nil {
		return fmt.Errorf("winget upgrade failed for %s: %w", id, runErr)
	}
	return fmt.Errorf("winget upgrade for %s was interrupted or closed before completion", id)
}

// UpgradePackage upgrades a specific package by its ID.
func UpgradePackage(ctx context.Context, id string) error {
	exe, err := wingetPath()
	if err != nil {
		return err
	}

	args := buildUpgradeArgs(id)
	cleanId := strings.ToLower(strings.TrimSpace(id))
	if strings.HasPrefix(cleanId, "microsoft.powershell") {
		fmt.Println("Note: Upgrading PowerShell requires Windows Installer to close running PowerShell sessions.")
		fmt.Println("Launching upgrade in an isolated console window so progress and errors are preserved when this shell closes.")
		fmt.Println("IMPORTANT: Do not open a new PowerShell window until the isolated installer window finishes.")
		return runIsolatedUpgrade(ctx, exe, id, args)
	} else if strings.HasPrefix(cleanId, "microsoft.windowsterminal") {
		fmt.Println("Note: Upgrading Windows Terminal will close or restart your terminal session.")
		fmt.Println("Launching upgrade in an isolated console window so progress and errors are preserved.")
		return runIsolatedUpgrade(ctx, exe, id, args)
	}

	cmd := exec.CommandContext(ctx, exe, args...)

	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
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

// UpgradeAll upgrades all available packages, deferring self-updating shell or terminal
// packages so they run last in an isolated console window.
func UpgradeAll(ctx context.Context) error {
	updates, err := GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		fmt.Println("No applicable winget updates found.")
		return nil
	}

	var regular, deferred []string
	for _, u := range updates {
		if shouldOmitSilent(u.Id) {
			deferred = append(deferred, u.Id)
		} else {
			regular = append(regular, u.Id)
		}
	}

	var hadErr bool
	for _, id := range regular {
		fmt.Printf("\n--- Upgrading: %s (winget) ---\n", id)
		if err := UpgradePackage(ctx, id); err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", id, err)
			hadErr = true
		}
	}
	for _, id := range deferred {
		fmt.Printf("\n--- Upgrading: %s (winget) ---\n", id)
		if err := UpgradePackage(ctx, id); err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", id, err)
			hadErr = true
		}
	}

	if hadErr {
		return fmt.Errorf("one or more winget package upgrades failed")
	}
	return nil
}
