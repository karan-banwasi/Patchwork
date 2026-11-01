//go:build !windows

package choco

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// isElevated returns true on non-Windows operating systems.
func isElevated() bool {
	return true
}

func trustedChocoPath() (string, error) {
	return "", errors.New("trusted choco.exe resolution is only supported on Windows")
}

func formatElevatedArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = pm.EscapeWindowsArg(a)
	}
	return strings.Join(quoted, " ")
}

func runElevated(ctx context.Context, exe string, args ...string) error {
	return fmt.Errorf("UAC elevation is only supported on Windows")
}
