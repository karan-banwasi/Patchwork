package brew

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
	_ pm.Lister    = (*BrewManager)(nil)
	_ pm.Installer = (*BrewManager)(nil)
)

type brewInstalledV2 struct {
	Formulae []struct {
		Name      string `json:"name"`
		Installed []struct {
			Version string `json:"version"`
		} `json:"installed"`
		LinkedKeg string `json:"linked_keg"`
	} `json:"formulae"`
	Casks []struct {
		Token     string      `json:"token"`
		Name      interface{} `json:"name"`
		Installed string      `json:"installed"`
	} `json:"casks"`
}

// ListInstalled runs `brew info --json=v2 --installed` and parses installed formulae and casks.
func (b *BrewManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := b.Path()
	if err != nil {
		return nil, err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"info", "--json=v2", "--installed"}, nil, &stdout, &stderr); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("brew info failed: %w (%s)", err, stderrStr)
		}
		return nil, fmt.Errorf("brew info failed: %w", err)
	}

	return parseBrewInstalledJSON(stdout.String())
}

func parseBrewInstalledJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "{}" {
		return []pm.InstalledPackage{}, nil
	}

	var root brewInstalledV2
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return nil, fmt.Errorf("failed to parse brew info JSON: %w", err)
	}

	var pkgs []pm.InstalledPackage
	for _, f := range root.Formulae {
		id := strings.TrimSpace(f.Name)
		if id == "" || pm.ValidateBrewID(id) != nil {
			continue
		}
		ver := strings.TrimSpace(f.LinkedKeg)
		if ver == "" && len(f.Installed) > 0 {
			ver = strings.TrimSpace(f.Installed[len(f.Installed)-1].Version)
		}
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        id,
			ID:          id,
			Version:     ver,
			ManagerName: "brew",
		})
	}

	for _, c := range root.Casks {
		id := strings.TrimSpace(c.Token)
		if id == "" || pm.ValidateBrewID(id) != nil {
			continue
		}
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        extractCaskName(c.Name, id),
			ID:          id,
			Version:     strings.TrimSpace(c.Installed),
			ManagerName: "brew",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a Homebrew formula or cask via `brew install <id>`.
func (b *BrewManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := b.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := b.Path()
	if err != nil {
		return err
	}
	runner := b.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := b.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := b.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if err := runner.Run(ctx, exe, []string{"install", id}, b.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("brew install failed for %s: %w", id, err)
	}
	return nil
}
