package winget

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// shouldOmitSilent returns true if the package is known to cause abrupt process
// termination or issues when upgraded in silent mode (such as the host shell or terminal).
func shouldOmitSilent(id string) bool {
	lower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasPrefix(lower, "microsoft.powershell") ||
		strings.HasPrefix(lower, "microsoft.windowsterminal") ||
		lower == "pwsh" ||
		lower == "powershell" ||
		lower == "windows-terminal"
}

// buildUpgradeArgs constructs the winget command-line arguments for upgrading a package (with agreement flags enabled).
func buildUpgradeArgs(id string) []string {
	return buildUpgradeArgsWithOptions(id, true)
}

// buildUpgradeArgsWithOptions constructs the winget command-line arguments for upgrading a package.
func buildUpgradeArgsWithOptions(id string, acceptAgreements bool) []string {
	args := []string{"upgrade", "--id", id, "--exact"}
	if !shouldOmitSilent(id) {
		args = append(args, "--silent")
	}
	if acceptAgreements {
		args = append(args, "--accept-source-agreements", "--accept-package-agreements")
	}
	return args
}

// buildIsolatedUpgradeScript constructs the batch script used to run self-updating shell
// or terminal upgrades in a standalone console window so that closing PowerShell or
// Windows Terminal does not kill winget mid-install or swallow installer errors.
func buildIsolatedUpgradeScript(wingetExe string, id string, args []string, statusFile string) string {
	quotedArgs := make([]string, len(args))
	for i, a := range args {
		quotedArgs[i] = pm.EscapeWindowsArg(a)
	}
	cmdLine := fmt.Sprintf(`%s %s`, pm.QuoteWindowsArg(wingetExe), strings.Join(quotedArgs, " "))
	safeDisplayID := pm.EscapeBatchText(id)

	lines := []string{
		"@echo off",
		fmt.Sprintf("title Patchwork - Upgrading %s", safeDisplayID),
		"echo ===============================================================================",
		fmt.Sprintf("echo  Patchwork: Upgrading %s", safeDisplayID),
		"echo  Windows Installer will close running PowerShell/Terminal sessions during setup.",
		"echo  IMPORTANT: Do NOT open a new PowerShell window until this window completes,",
		"echo  or Windows Installer will fail with MSI error 1603.",
		"echo ===============================================================================",
		"echo.",
		cmdLine,
		`set "EXIT_CODE=%ERRORLEVEL%"`,
		fmt.Sprintf(`> %s echo %%EXIT_CODE%%`, pm.QuoteWindowsArg(statusFile)),
		"echo.",
		`if NOT "%EXIT_CODE%"=="0" (`,
		"    echo ===============================================================================",
		fmt.Sprintf("    echo  [ERROR] winget upgrade for %s failed ^(exit code: %%EXIT_CODE%%^).", safeDisplayID),
		"    echo  If MSI failed with code 1603, another PowerShell session ^(such as an IDE",
		"    echo  terminal or extension^) was still running or reopened during the upgrade.",
		"    echo  Close all other PowerShell processes and retry.",
		"    echo ===============================================================================",
		"    pause",
		"    exit /b %EXIT_CODE%",
		")",
		"echo ===============================================================================",
		fmt.Sprintf("echo  [OK] Successfully upgraded %s!", safeDisplayID),
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
func (w *WingetManager) UpgradePackage(ctx context.Context, id string) error {
	if err := w.ValidateID(id); err != nil {
		return err
	}

	exe, err := wingetPath()
	if err != nil {
		return err
	}

	args := buildUpgradeArgsWithOptions(id, w.AcceptAgreements)
	cleanId := strings.ToLower(strings.TrimSpace(id))
	stdout := w.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := w.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if strings.HasPrefix(cleanId, "microsoft.powershell") {
		fmt.Fprintln(stdout, "Note: Upgrading PowerShell requires Windows Installer to close running PowerShell sessions.")
		fmt.Fprintln(stdout, "Launching upgrade in an isolated console window so progress and errors are preserved when this shell closes.")
		fmt.Fprintln(stdout, "IMPORTANT: Do not open a new PowerShell window until the isolated installer window finishes.")
		return runIsolatedUpgrade(ctx, exe, id, args)
	} else if strings.HasPrefix(cleanId, "microsoft.windowsterminal") {
		fmt.Fprintln(stdout, "Note: Upgrading Windows Terminal will close or restart your terminal session.")
		fmt.Fprintln(stdout, "Launching upgrade in an isolated console window so progress and errors are preserved.")
		return runIsolatedUpgrade(ctx, exe, id, args)
	}

	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, w.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget upgrade failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("winget upgrade failed: %w", err)
	}
	return nil
}

// UpgradePackage is a package-level helper for convenience.
func UpgradePackage(ctx context.Context, id string) error {
	return NewWingetManager().UpgradePackage(ctx, id)
}
