//go:build windows

package choco

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isElevated checks if the current process is running with Administrator privileges on Windows.
func isElevated() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

// runElevated executes the given executable and arguments in an elevated UAC console window
// using ShellExecuteExW ("runas") and waits for the process to exit.
func runElevated(exe string, args ...string) error {
	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}

	var quotedArgs []string
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			quotedArgs = append(quotedArgs, fmt.Sprintf(`"%s"`, strings.ReplaceAll(a, `"`, `\"`)))
		} else {
			quotedArgs = append(quotedArgs, a)
		}
	}
	paramPtr, err := syscall.UTF16PtrFromString(strings.Join(quotedArgs, " "))
	if err != nil {
		return err
	}

	type shellExecuteInfo struct {
		cbSize       uint32
		fMask        uint32
		hwnd          uintptr
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
		_, _ = windows.WaitForSingleObject(sei.hProcess, windows.INFINITE)
		var exitCode uint32
		if err := windows.GetExitCodeProcess(sei.hProcess, &exitCode); err == nil && exitCode != 0 {
			return fmt.Errorf("elevated choco process exited with code %d", exitCode)
		}
	}
	return nil
}
