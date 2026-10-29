//go:build windows

package winget

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// runIsolatedUpgrade executes winget in a standalone Win32 console host (conhost.exe)
// so that when Windows Restart Manager closes running pwsh.exe or WindowsTerminal.exe
// sessions during InstallValidate, winget.exe is not terminated and any installer errors
// remain visible to the user.
func runIsolatedUpgrade(ctx context.Context, exe string, id string, args []string) error {
	scriptFile, err := os.CreateTemp("", "patchwork-isolated-upgrade-*.cmd")
	if err != nil {
		return fmt.Errorf("failed to create temporary upgrade script: %w", err)
	}
	scriptPath := scriptFile.Name()
	_ = scriptFile.Close()

	statusFile, err := os.CreateTemp("", "patchwork-isolated-upgrade-*.status")
	if err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("failed to create temporary status file: %w", err)
	}
	statusPath := statusFile.Name()
	_ = statusFile.Close()
	_ = os.Remove(statusPath)

	scriptContent := buildIsolatedUpgradeScript(exe, id, args, statusPath)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("failed to write temporary upgrade script: %w", err)
	}
	defer func() {
		_ = os.Remove(scriptPath)
		_ = os.Remove(statusPath)
	}()

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	cmdExe := filepath.Join(sysRoot, "System32", "cmd.exe")
	if _, err := os.Stat(cmdExe); err != nil {
		if lookedUp, lookErr := exec.LookPath("cmd.exe"); lookErr == nil {
			cmdExe = lookedUp
		} else {
			cmdExe = "cmd.exe"
		}
	}

	cmd := exec.CommandContext(ctx, cmdExe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE | windows.CREATE_NEW_PROCESS_GROUP,
		CmdLine:       fmt.Sprintf(`"%s" /d /c "%s"`, cmdExe, scriptPath),
	}

	runErr := cmd.Run()

	statusBytes, readErr := os.ReadFile(statusPath)
	return parseIsolatedUpgradeStatus(id, string(statusBytes), readErr, runErr)
}
