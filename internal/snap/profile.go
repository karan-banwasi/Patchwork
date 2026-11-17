package snap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*SnapManager)(nil)
	_ pm.Installer = (*SnapManager)(nil)
)

// ListInstalled runs `snap list` and parses installed Snap packages.
func (s *SnapManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := s.Path()
	if err != nil {
		return nil, err
	}
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("snap list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("snap list failed: %w", err)
	}

	return parseSnapList(stdout.String()), nil
}

func parseSnapList(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "No snaps are installed") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			id := fields[0]
			ver := fields[1]
			if pm.ValidateSnapID(id) == nil {
				pkgs = append(pkgs, pm.InstalledPackage{
					Name:        id,
					ID:          id,
					Version:     ver,
					ManagerName: "snap",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a Snap package via `snap install <id>`.
func (s *SnapManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := s.Path()
	if err != nil {
		return err
	}
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := s.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := s.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	runExe, runArgs := wrapSudo(exe, []string{"install", id})
	if err := runner.Run(ctx, runExe, runArgs, s.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("snap install failed for %s: %w", id, err)
	}
	return nil
}
