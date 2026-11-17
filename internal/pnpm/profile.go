package pnpm

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
	_ pm.Lister    = (*PnpmManager)(nil)
	_ pm.Installer = (*PnpmManager)(nil)
)

type pnpmListProject struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// ListInstalled runs `pnpm list -g --depth=0 --json` and parses globally installed pnpm packages.
func (p *PnpmManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := p.Path()
	if err != nil {
		return nil, err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "-g", "--depth=0", "--json"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("pnpm list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("pnpm list failed: %w", err)
	}

	return parsePnpmListJSON(stdout.String())
}

func parsePnpmListJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
		return []pm.InstalledPackage{}, nil
	}

	var projects []pnpmListProject
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &projects); err != nil {
			return nil, fmt.Errorf("failed to parse pnpm list JSON: %w", err)
		}
	} else {
		var single pnpmListProject
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("failed to parse pnpm list JSON: %w", err)
		}
		projects = append(projects, single)
	}

	var pkgs []pm.InstalledPackage
	seen := make(map[string]bool)
	for _, proj := range projects {
		for name, entry := range proj.Dependencies {
			if pm.ValidateNpmID(name) != nil || seen[name] {
				continue
			}
			seen[name] = true
			pkgs = append(pkgs, pm.InstalledPackage{
				Name:        name,
				ID:          name,
				Version:     strings.TrimSpace(entry.Version),
				ManagerName: "pnpm",
			})
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a global pnpm package (optionally specifying version via `id@version`).
func (p *PnpmManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := p.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := p.Path()
	if err != nil {
		return err
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

	target := id
	if version != "" {
		target = id + "@" + version
	}
	if err := runner.Run(ctx, exe, []string{"add", "-g", target}, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pnpm install failed for %s: %w", id, err)
	}
	return nil
}
