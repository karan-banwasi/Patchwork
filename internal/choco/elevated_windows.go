//go:build windows

package choco

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// isElevated checks if the current process is running with Administrator privileges on Windows.
func isElevated() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

// trustedChocoPath resolves choco.exe strictly from machine-trusted locations
// (HKLM machine environment or FOLDERID_ProgramData) and never from PATH or user environment variables.
func trustedChocoPath() (string, error) {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.QUERY_VALUE,
	)
	if err == nil {
		defer k.Close()
		if dir, _, err := k.GetStringValue("ChocolateyInstall"); err == nil && dir != "" {
			p := filepath.Join(dir, "bin", "choco.exe")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	pd, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err == nil && pd != "" {
		p := filepath.Join(pd, "chocolatey", "bin", "choco.exe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", errors.New("trusted choco.exe not found in HKLM ChocolateyInstall or ProgramData; refusing to elevate")
}

// formatElevatedArgs quotes arguments using Windows CommandLineToArgvW rules.
func formatElevatedArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = pm.EscapeWindowsArg(a)
	}
	return strings.Join(quoted, " ")
}

// runElevated executes the given executable and arguments in an elevated UAC console window
// using ShellExecuteExW ("runas") and waits for the process to exit or ctx to be canceled.
func runElevated(ctx context.Context, exe string, args ...string) error {
	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	paramPtr, err := syscall.UTF16PtrFromString(formatElevatedArgs(args))
	if err != nil {
		return err
	}

	type shellExecuteInfo struct {
		cbSize       uint32
		fMask        uint32
		hwnd         uintptr
		lpVerb       *uint16
		lpFile       *uint16
		lpParameters *uint16
		lpDirectory  *uint16
		nShow        int32
		hInstApp     uintptr
		lpIDList     uintptr
		lpClass      *uint16
		hkeyClass    uintptr
		dwHotKey     uint32
		hIconOrMon   uintptr
		hProcess     windows.Handle
	}

	const seeMaskNoCloseProcess = 0x00000040
	const swShowNormal = 1

	sei := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verbPtr,
		lpFile:       exePtr,
		lpParameters: paramPtr,
		nShow:        swShowNormal,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW := shell32.NewProc("ShellExecuteExW")

	r1, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r1 == 0 {
		return fmt.Errorf("UAC elevation canceled or failed: %w", callErr)
	}
	if sei.hProcess != 0 {
		defer windows.CloseHandle(sei.hProcess)
		for {
			event, waitErr := windows.WaitForSingleObject(sei.hProcess, 250)
			if event == windows.WAIT_OBJECT_0 {
				break
			}
			if event != uint32(windows.WAIT_TIMEOUT) && waitErr != nil {
				return fmt.Errorf("waiting for elevated choco process failed: %w", waitErr)
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("waiting for elevated choco process canceled: %w", ctx.Err())
			default:
			}
		}
		var exitCode uint32
		if err := windows.GetExitCodeProcess(sei.hProcess, &exitCode); err == nil && exitCode != 0 {
			return fmt.Errorf("elevated choco process exited with code %d", exitCode)
		}
	}
	return nil
}
