//go:build !windows

package winget

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func runIsolatedUpgrade(ctx context.Context, exe string, id string, args []string) error {
	cmd := exec.CommandContext(ctx, exe, args...)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget upgrade failed: %w (%s)", err, stderrStr)
		}
		return fmt.Errorf("winget upgrade failed: %w", err)
	}
	return nil
}
