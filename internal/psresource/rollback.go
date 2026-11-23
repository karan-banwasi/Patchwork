package psresource

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var _ pm.Rollbackable = (*PSResourceManager)(nil)

// RollbackPackage installs a specific target version of a PowerShell PSResource module.
func (p *PSResourceManager) RollbackPackage(ctx context.Context, id, targetVersion string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateVersion(targetVersion); err != nil {
		return err
	}

	runner := p.runner
	exe, err := pwshPath()
	if err != nil {
		if !pm.IsExecRunner(runner) {
			exe = "pwsh"
		} else {
			return err
		}
	}
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	stdout := p.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := p.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	scriptBlock := "& { param([string]$n, [string]$v) Install-PSResource -Name $n -Version $v -Reinstall -Confirm:$false }"
	if p.TrustRepository {
		scriptBlock = "& { param([string]$n, [string]$v) Install-PSResource -Name $n -Version $v -Reinstall -TrustRepository -Confirm:$false }"
	}

	var stderrBuf bytes.Buffer
	args := []string{"-NoProfile", "-NonInteractive", "-Command", scriptBlock, id, targetVersion}
	if err := runner.Run(ctx, exe, args, p.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		errText := strings.TrimSpace(stderrBuf.String())
		if !p.TrustRepository && (strings.Contains(strings.ToLower(errText), "untrusted") || strings.Contains(strings.ToLower(errText), "trust")) {
			return fmt.Errorf("psresource rollback failed for %s@%s (repository is not trusted; run Set-PSResourceRepository -Trusted or pass --trust-repository): %w", id, targetVersion, err)
		}
		return fmt.Errorf("psresource rollback failed for %s@%s: %w", id, targetVersion, err)
	}
	return nil
}
