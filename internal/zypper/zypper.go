package zypper

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
	_ pm.Manager        = (*ZypperManager)(nil)
	_ pm.Locator        = (*ZypperManager)(nil)
	_ pm.Cleaner        = (*ZypperManager)(nil)
	_ pm.Refresher      = (*ZypperManager)(nil)
	_ pm.BulkUpgrader   = (*ZypperManager)(nil)
	_ pm.IOConfigurable = (*ZypperManager)(nil)
)

// ZypperManager implements the pm.Manager interface for openSUSE / SUSE zypper packages.
type ZypperManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewZypperManager() *ZypperManager {
	return &ZypperManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (z *ZypperManager) Name() string {
	return "zypper"
}

func (z *ZypperManager) Aliases() []string {
	return []string{"opensuse", "suse"}
}

func (z *ZypperManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	z.stdin = stdin
	if stdout != nil {
		z.stdout = stdout
	} else {
		z.stdout = io.Discard
	}
	if stderr != nil {
		z.stderr = stderr
	} else {
		z.stderr = io.Discard
	}
}

func (z *ZypperManager) ValidateID(id string) error {
	return pm.ValidateZypperID(id)
}

func (z *ZypperManager) Path() (string, error) {
	if z.exePath != "" {
		return z.exePath, nil
	}
	return zypperPath()
}

func (z *ZypperManager) Version(ctx context.Context) (string, error) {
	exe, err := z.Path()
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

func (z *ZypperManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := z.Path()
	return err == nil
}

func zypperPath() (string, error) {
	for _, name := range []string{"zypper"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/zypper"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("zypper executable not found")
}

func wrapSudo(cmd string, args []string) (string, []string) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil {
			return sudoPath, append([]string{cmd}, args...)
		}
	}
	return cmd, args
}

func (z *ZypperManager) Refresh(ctx context.Context) error {
	exe, err := z.Path()
	if err != nil {
		return err
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	runExe, runArgs := wrapSudo(exe, []string{"refresh"})
	return runner.Run(ctx, runExe, runArgs, z.stdin, z.stdout, z.stderr)
}

func (z *ZypperManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := z.Path()
	if err != nil {
		return nil
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up zypper cache ---")
	runExe, runArgs := wrapSudo(exe, []string{"clean", "--all"})
	return runner.Run(ctx, runExe, runArgs, z.stdin, out, errOut)
}

func (z *ZypperManager) UpgradePackage(ctx context.Context, id string) error {
	return z.UpgradePackages(ctx, []string{id})
}

func (z *ZypperManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := z.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := z.Path()
	if err != nil {
		return err
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := z.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := z.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"--non-interactive", "update"}, ids...)
	runExe, runArgs := wrapSudo(exe, args)
	if err := runner.Run(ctx, runExe, runArgs, z.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("zypper update failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (z *ZypperManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := z.Path()
	if err != nil {
		return nil, err
	}
	runner := z.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"--terse", "list-updates"}, nil, &stdout, &stderr)
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if strings.Contains(err.Error(), "100") {
			exitCode = 100
		} else if strings.Contains(err.Error(), "101") {
			exitCode = 101
		}
		// zypper returns 0 (no updates), 100 (updates available), 101 (updates available, restart suggested)
		if exitCode != 100 && exitCode != 101 {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return nil, fmt.Errorf("zypper list-updates failed: %w (%s)", err, errMsg)
			}
			return nil, fmt.Errorf("zypper list-updates failed: %w", err)
		}
	}

	return parseZypperListUpdates(stdout.String())
}

func parseZypperListUpdates(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}

		// Header line has "Name" or "Current Version"
		if strings.EqualFold(parts[2], "Name") || strings.EqualFold(parts[3], "Current Version") {
			continue
		}

		repo := parts[1]
		id := parts[2]
		curVer := parts[3]
		availVer := parts[4]

		if pm.ValidateZypperID(id) != nil {
			continue
		}

		if version.IsUpgrade("zypper", curVer, availVer) {
			updates = append(updates, pm.PackageUpdate{
				Name:             id,
				Id:               id,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           repo,
				ManagerName:      "zypper",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
