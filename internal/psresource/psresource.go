package psresource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// PSResourceManager implements the pm.Manager interface for PowerShell modules via Microsoft.PowerShell.PSResourceGet.
type PSResourceManager struct {
	TrustRepository bool
	runner          pm.Runner
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
}

func NewPSResourceManager() *PSResourceManager {
	return &PSResourceManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (p *PSResourceManager) Name() string {
	return "psresource"
}

func (p *PSResourceManager) Aliases() []string {
	return []string{"powershell", "psresourceget"}
}

func (p *PSResourceManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	p.stdin = stdin
	if stdout != nil {
		p.stdout = stdout
	} else {
		p.stdout = io.Discard
	}
	if stderr != nil {
		p.stderr = stderr
	} else {
		p.stderr = io.Discard
	}
}

func (p *PSResourceManager) ValidateID(id string) error {
	return pm.ValidatePSResourceID(id)
}

func (p *PSResourceManager) Path() (string, error) {
	return pwshPath()
}

func (p *PSResourceManager) Version(ctx context.Context) (string, error) {
	if _, err := pwshPath(); err != nil {
		return "", err
	}
	return "PSResourceGet", nil
}

func (p *PSResourceManager) IsAvailable(ctx context.Context) bool {
	_, err := pwshPath()
	return err == nil
}

func pwshPath() (string, error) {
	for _, name := range []string{"pwsh.exe", "pwsh", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("PowerShell executable not found")
}

type psResourceUpdateJSON struct {
	Name      string `json:"Name"`
	Id        string `json:"Id"`
	Version   string `json:"Version"`
	Available string `json:"Available"`
	Source    string `json:"Source"`
}

const checkPSResourceScript = `
$ErrorActionPreference = 'Stop'
try {
    if (-not (Get-Command Get-InstalledPSResource -ErrorAction SilentlyContinue)) { exit 0 }
    $installed = Get-InstalledPSResource -Scope CurrentUser | Group-Object Name | ForEach-Object { $_.Group | Sort-Object {[version]($_.Version -replace '-.*$','')} -Descending | Select-Object -First 1 }
    if (-not $installed) { exit 0 }
    $names = [string[]]($installed | Select-Object -ExpandProperty Name)
    $found = Find-PSResource -Name $names | Group-Object Name | ForEach-Object { $_.Group | Select-Object -First 1 }
    $foundMap = @{}
    foreach ($f in $found) { $foundMap[$f.Name.ToLower()] = $f }
    $updates = @()
    foreach ($pkg in $installed) {
        $f = $foundMap[$pkg.Name.ToLower()]
        if ($f -and ([string]$f.Version -ne [string]$pkg.Version)) {
            $updates += [PSCustomObject]@{
                Name      = [string]$pkg.Name
                Id        = [string]$pkg.Name
                Version   = [string]$pkg.Version
                Available = [string]$f.Version
                Source    = if ($pkg.Repository) { [string]$pkg.Repository } else { 'PSGallery' }
            }
        }
    }
    if ($updates.Count -gt 0) {
        $updates | ConvertTo-Json -Compress
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`

func (p *PSResourceManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := pwshPath()
	if err != nil {
		return nil, err
	}

	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	runErr := runner.Run(ctx, exe, []string{"-NoProfile", "-NonInteractive", "-Command", checkPSResourceScript}, nil, &stdout, &stderr)
	outStr := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	if runErr != nil && outStr == "" {
		if errStr != "" {
			return nil, fmt.Errorf("psresource check failed: %w (%s)", runErr, errStr)
		}
		return nil, fmt.Errorf("psresource check failed: %w", runErr)
	}

	return parsePSResourceJSON(outStr)
}

func parsePSResourceJSON(output string) ([]pm.PackageUpdate, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return []pm.PackageUpdate{}, nil
	}

	var entries []psResourceUpdateJSON
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, fmt.Errorf("parse psresource output: %w", err)
		}
	} else if strings.HasPrefix(trimmed, "{") {
		var single psResourceUpdateJSON
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("parse psresource output: %w", err)
		}
		entries = append(entries, single)
	} else {
		return nil, fmt.Errorf("parse psresource output: unexpected non-JSON output")
	}

	var updates []pm.PackageUpdate
	for _, e := range entries {
		if e.Name != "" && e.Version != "" && e.Available != "" && e.Version != e.Available && pm.ValidatePSResourceID(e.Name) == nil {
			src := e.Source
			if src == "" {
				src = "PSGallery"
			}
			updates = append(updates, pm.PackageUpdate{
				Name:             e.Name,
				Id:               e.Name,
				Version:          e.Version,
				AvailableVersion: e.Available,
				Source:           src,
				ManagerName:      "psresource",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})
	return updates, nil
}

func (p *PSResourceManager) UpgradePackage(ctx context.Context, id string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	exe, err := pwshPath()
	if err != nil {
		return err
	}

	scriptBlock := "& { param([string]$n) Update-PSResource -Name $n -Confirm:$false }"
	if p.TrustRepository {
		scriptBlock = "& { param([string]$n) Update-PSResource -Name $n -TrustRepository -Confirm:$false }"
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
	args := []string{"-NoProfile", "-NonInteractive", "-Command", scriptBlock, id}
	if err := runner.Run(ctx, exe, args, p.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		errText := strings.TrimSpace(stderrBuf.String())
		if !p.TrustRepository && (strings.Contains(strings.ToLower(errText), "untrusted") || strings.Contains(strings.ToLower(errText), "trust")) {
			return fmt.Errorf("psresource update failed for %s (repository is not trusted; run Set-PSResourceRepository -Trusted or pass --trust-repository): %w", id, err)
		}
		return fmt.Errorf("psresource update failed for %s: %w", id, err)
	}
	return nil
}
