package psresource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// PSResourceManager implements the pm.Manager interface for PowerShell modules via Microsoft.PowerShell.PSResourceGet.
type PSResourceManager struct{}

func NewPSResourceManager() *PSResourceManager {
	return &PSResourceManager{}
}

func (p *PSResourceManager) Name() string {
	return "psresource"
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
$ErrorActionPreference = 'SilentlyContinue'
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
`

func (p *PSResourceManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := pwshPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", checkPSResourceScript)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run()

	return parsePSResourceJSON(stdout.String())
}

func parsePSResourceJSON(output string) ([]pm.PackageUpdate, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return []pm.PackageUpdate{}, nil
	}

	var entries []psResourceUpdateJSON
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return []pm.PackageUpdate{}, nil
		}
	} else if strings.HasPrefix(trimmed, "{") {
		var single psResourceUpdateJSON
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return []pm.PackageUpdate{}, nil
		}
		entries = append(entries, single)
	} else {
		return []pm.PackageUpdate{}, nil
	}

	var updates []pm.PackageUpdate
	for _, e := range entries {
		if e.Name != "" && e.Version != "" && e.Available != "" && e.Version != e.Available {
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
	exe, err := pwshPath()
	if err != nil {
		return err
	}
	safeID := strings.ReplaceAll(id, "'", "''")
	script := fmt.Sprintf("Update-PSResource -Name '%s' -TrustRepository -Confirm:$false", safeID)
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psresource update failed for %s: %w", id, err)
	}
	return nil
}

func (p *PSResourceManager) UpgradeAll(ctx context.Context) error {
	updates, err := p.GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	for _, u := range updates {
		if err := p.UpgradePackage(ctx, u.Id); err != nil {
			return err
		}
	}
	return nil
}
