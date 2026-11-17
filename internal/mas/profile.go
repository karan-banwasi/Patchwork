package mas

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*MasManager)(nil)
	_ pm.Installer = (*MasManager)(nil)
)

var masListRegex = regexp.MustCompile(`^([0-9]+)\s+(.+?)\s+\(([^)]+)\)$`)

// ListInstalled runs `mas list` and parses installed Mac App Store applications.
func (m *MasManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := m.Path()
	if err != nil {
		return nil, err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("mas list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("mas list failed: %w", err)
	}

	return parseMasList(stdout.String()), nil
}

func parseMasList(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Warning:") {
			continue
		}
		match := masListRegex.FindStringSubmatch(line)
		if len(match) == 4 {
			id := strings.TrimSpace(match[1])
			name := strings.TrimSpace(match[2])
			ver := strings.TrimSpace(match[3])
			if pm.ValidateMasID(id) == nil {
				pkgs = append(pkgs, pm.InstalledPackage{
					Name:        name,
					ID:          id,
					Version:     ver,
					ManagerName: "mas",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].Name) < strings.ToLower(pkgs[j].Name)
	})
	return pkgs
}

// InstallPackage installs a Mac App Store application by numeric ID.
func (m *MasManager) InstallPackage(ctx context.Context, id, version string) error {
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

	if err := runner.Run(ctx, exe, []string{"install", id}, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mas install failed for %s: %w", id, err)
	}
	return nil
}
