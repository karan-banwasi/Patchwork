package scoop

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
	_ pm.Lister    = (*ScoopManager)(nil)
	_ pm.Installer = (*ScoopManager)(nil)
)

func isMockRunner(r pm.Runner) bool {
	if r == nil {
		return false
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return !isExec && !isExecPtr
}

func (s *ScoopManager) resolveExe() (string, error) {
	exe, err := scoopPath()
	if err != nil {
		if isMockRunner(s.runner) {
			return "scoop", nil
		}
		return "", err
	}
	return exe, nil
}

type scoopExportOutput struct {
	Apps []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
		Source  string `json:"Source"`
	} `json:"apps"`
}

// ListInstalled runs `scoop export` to list installed Scoop packages.
func (s *ScoopManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := s.resolveExe()
	if err != nil {
		return nil, err
	}

	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"export"}, nil, &stdout, &stderr); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("scoop export failed: %w (%s)", err, stderrStr)
		}
		return nil, fmt.Errorf("scoop export failed: %w", err)
	}

	return parseScoopExportJSON(stdout.String())
}

func parseScoopExportJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "{}" {
		return []pm.InstalledPackage{}, nil
	}

	var data scoopExportOutput
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return nil, fmt.Errorf("parse scoop export JSON: %w", err)
	}

	var pkgs []pm.InstalledPackage
	for _, app := range data.Apps {
		name := strings.TrimSpace(app.Name)
		ver := strings.TrimSpace(app.Version)
		if name == "" || pm.ValidateScoopID(name) != nil {
			continue
		}
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        name,
			ID:          name,
			Version:     ver,
			ManagerName: "scoop",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a Scoop package with optional version (`id@version`).
func (s *ScoopManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := s.resolveExe()
	if err != nil {
		return err
	}

	target := id
	if version != "" {
		target = id + "@" + version
	}

	stdout := s.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := s.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"install", target}, s.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("scoop install failed for %s: %w (%s)", id, err, stderrStr)
		}
		return fmt.Errorf("scoop install failed for %s: %w", id, err)
	}
	return nil
}
