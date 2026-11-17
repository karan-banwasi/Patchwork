package winget

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
	_ pm.Lister    = (*WingetManager)(nil)
	_ pm.Installer = (*WingetManager)(nil)
)

func (w *WingetManager) resolveExe() (string, error) {
	exe, err := wingetPath()
	if err != nil {
		if !pm.IsExecRunner(w.runner) {
			return "winget", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `winget list --accept-source-agreements` and parses installed packages.
func (w *WingetManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := w.resolveExe()
	if err != nil {
		return nil, err
	}

	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"list", "--accept-source-agreements"}, nil, &stdout, &stderr)
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	if strings.Contains(outputStr, "No installed package found matching input criteria.") ||
		strings.Contains(outputStr, "No packages found") {
		return []pm.InstalledPackage{}, nil
	}

	pkgs, parseErr := parseWingetListOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	if err != nil && len(pkgs) == 0 {
		errMsg := fmt.Sprintf("winget list failed: %v", err)
		if stderrStr != "" {
			errMsg += fmt.Sprintf(" (stderr: %s)", stderrStr)
		} else if strings.TrimSpace(outputStr) != "" {
			errMsg += fmt.Sprintf(" (stdout: %s)", strings.TrimSpace(outputStr))
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return pkgs, nil
}

func parseWingetListOutput(output string) ([]pm.InstalledPackage, error) {
	rows, err := parseWingetTable(output, false)
	if err != nil {
		return nil, err
	}
	pkgs := make([]pm.InstalledPackage, 0, len(rows))
	for _, r := range rows {
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        r.name,
			ID:          r.id,
			Version:     r.version,
			ManagerName: "winget",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a specific winget package ID and optional version.
func (w *WingetManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := w.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := w.resolveExe()
	if err != nil {
		return err
	}

	args := []string{"install", "--id", id, "--exact"}
	if version != "" {
		args = append(args, "--version", version)
	}
	if w.AcceptAgreements {
		if !shouldOmitSilent(id) {
			args = append(args, "--silent")
		}
		args = append(args, "--accept-source-agreements", "--accept-package-agreements")
	}

	stdout := w.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := w.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, w.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget install failed for %s: %w (%s)", id, err, stderrStr)
		}
		return fmt.Errorf("winget install failed for %s: %w", id, err)
	}
	return nil
}
