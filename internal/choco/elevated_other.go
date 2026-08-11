//go:build !windows

package choco

// isElevated returns true on non-Windows operating systems.
func isElevated() bool {
	return true
}
