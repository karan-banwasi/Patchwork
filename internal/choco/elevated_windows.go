//go:build windows

package choco

import (
	"golang.org/x/sys/windows"
)

// isElevated checks if the current process is running with Administrator privileges on Windows.
func isElevated() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}
