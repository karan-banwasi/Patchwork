//go:build !windows

package choco

import "fmt"

// isElevated returns true on non-Windows operating systems.
func isElevated() bool {
	return true
}

func runElevated(exe string, args ...string) error {
	return fmt.Errorf("UAC elevation is only supported on Windows")
}
