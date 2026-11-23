package mise

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
	_ pm.Lister    = (*MiseManager)(nil)
	_ pm.Installer = (*MiseManager)(nil)
)

type miseLsEntry struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Active    bool   `json:"active"`
	Installed bool   `json:"installed"`
}

// ListInstalled runs `mise ls --json` and parses installed mise tools.
func (m *MiseManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := m.Path()
	if err != nil {
		return nil, err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"ls", "--json"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("mise ls failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("mise ls failed: %w", err)
	}

	return parseMiseLsJSON(stdout.String())
}

func parseMiseLsJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "{}" || trimmed == "[]" {
		return []pm.InstalledPackage{}, nil
	}

	byID := make(map[string]miseLsEntry)

	if strings.HasPrefix(trimmed, "[") {
		var list []miseLsEntry
		if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
			return nil, fmt.Errorf("failed to parse mise ls JSON: %w", err)
		}
		for _, entry := range list {
			id := strings.TrimSpace(entry.Name)
			if id == "" || pm.ValidateMiseID(id) != nil {
				continue
			}
			prev, exists := byID[id]
			if !exists || (entry.Active && !prev.Active) {
				byID[id] = entry
			}
		}
	} else {
		var mapped map[string][]miseLsEntry
		if err := json.Unmarshal([]byte(trimmed), &mapped); err != nil {
			return nil, fmt.Errorf("failed to parse mise ls JSON: %w", err)
		}
		for tool, entries := range mapped {
			id := strings.TrimSpace(tool)
			if id == "" || pm.ValidateMiseID(id) != nil {
				continue
			}
			for _, entry := range entries {
				prev, exists := byID[id]
				if !exists || (entry.Active && !prev.Active) {
					byID[id] = entry
				}
			}
		}
	}

	pkgs := make([]pm.InstalledPackage, 0, len(byID))
	for id, entry := range byID {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        id,
			ID:          id,
			Version:     strings.TrimSpace(entry.Version),
			ManagerName: "mise",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs and activates a global mise tool via `mise use -g <id>[@<version>]`.
func (m *MiseManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := m.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := m.Path()
	if err != nil {
		return err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := m.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := m.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "@" + version
	}
	if err := runner.Run(ctx, exe, []string{"use", "-g", target}, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mise use -g failed for %s: %w", id, err)
	}
	return nil
}
