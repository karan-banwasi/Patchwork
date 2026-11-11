package snap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*SnapManager)(nil)
	_ pm.Locator        = (*SnapManager)(nil)
	_ pm.BulkUpgrader   = (*SnapManager)(nil)
	_ pm.IOConfigurable = (*SnapManager)(nil)
)

// SnapManager implements the pm.Manager interface for Canonical Snap packages on Linux.
type SnapManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewSnapManager() *SnapManager {
	return &SnapManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (s *SnapManager) Name() string {
	return "snap"
}

func (s *SnapManager) Aliases() []string {
	return []string{"snapcraft", "snappy"}
}

func (s *SnapManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	s.stdin = stdin
	if stdout != nil {
		s.stdout = stdout
	} else {
		s.stdout = io.Discard
	}
	if stderr != nil {
		s.stderr = stderr
	} else {
		s.stderr = io.Discard
	}
}

func (s *SnapManager) ValidateID(id string) error {
	return pm.ValidateSnapID(id)
}

func (s *SnapManager) Path() (string, error) {
	if s.exePath != "" {
		return s.exePath, nil
	}
	return snapPath()
}

func (s *SnapManager) Version(ctx context.Context) (string, error) {
	exe, err := s.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

func (s *SnapManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := s.Path()
	return err == nil
}

func snapPath() (string, error) {
	for _, name := range []string{"snap"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/snap"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("snap executable not found")
}

func wrapSudo(cmd string, args []string) (string, []string) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil {
			return sudoPath, append([]string{cmd}, args...)
		}
	}
	return cmd, args
}

func (s *SnapManager) UpgradePackage(ctx context.Context, id string) error {
	return s.UpgradePackages(ctx, []string{id})
}

func (s *SnapManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := s.ValidateID(id); err != nil {
			return err
		}
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

	args := append([]string{"refresh"}, ids...)
	runExe, runArgs := wrapSudo(exe, args)
	if err := runner.Run(ctx, runExe, runArgs, s.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("snap refresh failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (s *SnapManager) queryInstalledSnaps(ctx context.Context, exe string, runner pm.Runner) map[string]string {
	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list"}, nil, &stdout, &stderr); err != nil {
		return nil
	}
	installed := make(map[string]string)
	lines := strings.Split(stdout.String(), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 {
			installed[fields[0]] = fields[1]
		}
	}
	return installed
}

func (s *SnapManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := s.Path()
	if err != nil {
		return nil, err
	}
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"refresh", "--list"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("snap refresh --list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("snap refresh --list failed: %w", err)
	}

	installedMap := s.queryInstalledSnaps(ctx, exe, runner)
	return parseSnapRefreshList(stdout.String(), installedMap)
}

func parseSnapRefreshList(output string, installedMap map[string]string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "All snaps up to date") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		id := fields[0]
		availVer := fields[1]

		if pm.ValidateSnapID(id) != nil {
			continue
		}

		curVer := "unknown"
		if installedMap != nil {
			if v, ok := installedMap[id]; ok {
				curVer = v
			}
		}

		if curVer == "unknown" || version.IsUpgrade("snap", curVer, availVer) {
			updates = append(updates, pm.PackageUpdate{
				Name:             id,
				Id:               id,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "snapcraft",
				ManagerName:      "snap",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
