package psresource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*PSResourceManager)(nil)
	_ pm.Installer = (*PSResourceManager)(nil)
)

func isMockRunner(r pm.Runner) bool {
	if r == nil {
		return false
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return !isExec && !isExecPtr
}

func (p *PSResourceManager) resolveExe() (string, error) {
	exe, err := pwshPath()
	if err != nil {
		if isMockRunner(p.runner) {
			return "pwsh", nil
		}
		return "", err
	}
	return exe, nil
}

const listPSResourceScript = `
$ErrorActionPreference = 'Stop'
try {
    if (-not (Get-Command Get-InstalledPSResource -ErrorAction SilentlyContinue)) { exit 0 }
    $rawInstalled = try {
        Get-InstalledPSResource -Scope CurrentUser -ErrorAction Stop
    } catch {
        if ($_.FullyQualifiedErrorId -like "*InstalledPackageNotFound*" -or $_.Exception.Message -like "*No match was found*") {
            $null
        } else {
            throw
        }
    }
    if (-not $rawInstalled) { exit 0 }
    $installed = $rawInstalled | Group-Object Name | ForEach-Object { $_.Group | Sort-Object {[version]($_.Version -replace '-.*$','')} -Descending | Select-Object -First 1 }
    $list = @()
    foreach ($pkg in $installed) {
        $list += [PSCustomObject]@{
            Name    = [string]$pkg.Name
            Version = [string]$pkg.Version
        }
    }
    if ($list.Count -gt 0) {
        $list | ConvertTo-Json -Compress
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`

type psResourceInstalledJSON struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

// ListInstalled queries installed CurrentUser PowerShell PSResource modules.
func (p *PSResourceManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := p.resolveExe()
	if err != nil {
		return nil, err
	}

	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	runErr := runner.Run(ctx, exe, []string{"-NoProfile", "-NonInteractive", "-Command", listPSResourceScript}, nil, &stdout, &stderr)
	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	if runErr != nil && outStr == "" {
		if errStr != "" {
			return nil, fmt.Errorf("psresource list failed: %w (%s)", runErr, errStr)
		}
		return nil, fmt.Errorf("psresource list failed: %w", runErr)
	}

	return parsePSResourceInstalledJSON(outStr)
}

func parsePSResourceInstalledJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return []pm.InstalledPackage{}, nil
	}

	var entries []psResourceInstalledJSON
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, fmt.Errorf("parse psresource installed output: %w", err)
		}
	} else if strings.HasPrefix(trimmed, "{") {
		var single psResourceInstalledJSON
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("parse psresource installed output: %w", err)
		}
		entries = append(entries, single)
	} else {
		return nil, fmt.Errorf("parse psresource installed output: unexpected non-JSON output")
	}

	var pkgs []pm.InstalledPackage
	for _, e := range entries {
		name := strings.TrimSpace(e.Name)
		if name != "" && pm.ValidatePSResourceID(name) == nil {
			pkgs = append(pkgs, pm.InstalledPackage{
				Name:        name,
				ID:          name,
				Version:     strings.TrimSpace(e.Version),
				ManagerName: "psresource",
			})
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a PowerShell module via Install-PSResource.
func (p *PSResourceManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := p.resolveExe()
	if err != nil {
		return err
	}

	var scriptBlock string
	var args []string
	trustFlag := ""
	if p.TrustRepository {
		trustFlag = " -TrustRepository"
	}
	if version != "" {
		scriptBlock = fmt.Sprintf("& { param([string]$n, [string]$v) Install-PSResource -Name $n -Version $v -Scope CurrentUser%s -Confirm:$false }", trustFlag)
		args = []string{"-NoProfile", "-NonInteractive", "-Command", scriptBlock, id, version}
	} else {
		scriptBlock = fmt.Sprintf("& { param([string]$n) Install-PSResource -Name $n -Scope CurrentUser%s -Confirm:$false }", trustFlag)
		args = []string{"-NoProfile", "-NonInteractive", "-Command", scriptBlock, id}
	}

	runner := p.runner
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

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, p.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		errText := strings.TrimSpace(stderrBuf.String())
		if !p.TrustRepository && (strings.Contains(strings.ToLower(errText), "untrusted") || strings.Contains(strings.ToLower(errText), "trust")) {
			return fmt.Errorf("psresource install failed for %s (repository is not trusted; run Set-PSResourceRepository -Trusted or pass --trust-repository): %w", id, err)
		}
		return fmt.Errorf("psresource install failed for %s: %w", id, err)
	}
	return nil
}
