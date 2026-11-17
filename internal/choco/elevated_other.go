//go:build !windows

package choco

import (
	"context"
	"errors"
	"fmt"
)

// isElevated returns true on non-Windows operating systems.
func isElevated() bool {
	return true
}

func trustedChocoPath() (string, error) {
	return "", errors.New("trusted choco.exe resolution is only supported on Windows")
}

func runElevated(ctx context.Context, exe string, args ...string) error {
	return fmt.Errorf("UAC elevation is only supported on Windows")
}
