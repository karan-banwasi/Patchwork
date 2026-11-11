package pacman

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*PacmanManager)(nil)
	_ pm.Locator        = (*PacmanManager)(nil)
	_ pm.Cleaner        = (*PacmanManager)(nil)
	_ pm.Refresher      = (*PacmanManager)(nil)
	_ pm.BulkUpgrader   = (*PacmanManager)(nil)
	_ pm.IOConfigurable = (*PacmanManager)(nil)
)

// PacmanManager implements the pm.Manager interface for Arch Linux pacman packages.
type PacmanManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewPacmanManager() *PacmanManager {
	return &PacmanManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (p *PacmanManager) Name() string {
	return "pacman"
}

func (p *PacmanManager) Aliases() []string {
	return []string{"arch"}
}

func (p *PacmanManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	p.stdin = stdin
	if stdout != nil {
		p.stdout = stdout
	} else {
		p.stdout = io.Discard
	}
	if stderr != nil {
		p.stderr = stderr
	} else {
		p.stderr = io.Discard
	}
}

func (p *PacmanManager) ValidateID(id string) error {
	return pm.ValidatePacmanID(id)
}

func (p *PacmanManager) Path() (string, error) {
	if p.exePath != "" {
		return p.exePath, nil
	}
	return pacmanPath()
}

func (p *PacmanManager) Version(ctx context.Context) (string, error) {
	exe, err := p.Path()
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

func (p *PacmanManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := p.Path()
	return err == nil
}

func pacmanPath() (string, error) {
	for _, name := range []string{"pacman"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/pacman"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("pacman executable not found")
}

func wrapSudo(cmd string, args []string) (string, []string) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil {
			return sudoPath, append([]string{cmd}, args...)
		}
	}
	return cmd, args
}

func (p *PacmanManager) Refresh(ctx context.Context) error {
	exe, err := p.Path()
	if err != nil {
		return err
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	runExe, runArgs := wrapSudo(exe, []string{"-Sy"})
	return runner.Run(ctx, runExe, runArgs, p.stdin, p.stdout, p.stderr)
}

func (p *PacmanManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := p.Path()
	if err != nil {
		return nil
	}
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up pacman package cache ---")
	runExe, runArgs := wrapSudo(exe, []string{"-Sc", "--noconfirm"})
	return runner.Run(ctx, runExe, runArgs, p.stdin, out, errOut)
}

func (p *PacmanManager) UpgradePackage(ctx context.Context, id string) error {
	return p.UpgradePackages(ctx, []string{id})
}

func (p *PacmanManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := p.ValidateID(id); err != nil {
			return err
		}
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

	args := append([]string{"-S", "--noconfirm"}, ids...)
	runExe, runArgs := wrapSudo(exe, args)
	if err := runner.Run(ctx, runExe, runArgs, p.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("pacman upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (p *PacmanManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	runner := p.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	// Try checkupdates first (preferred for Arch because it doesn't touch local db)
	checkExe, err := exec.LookPath("checkupdates")
	var stdout, stderr bytes.Buffer

	if err == nil {
		runErr := runner.Run(ctx, checkExe, nil, nil, &stdout, &stderr)
		if runErr != nil {
			exitCode := -1
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if strings.Contains(runErr.Error(), "2") {
				exitCode = 2
			}
			// exit code 2 means up to date
			if exitCode != 2 {
				errMsg := strings.TrimSpace(stderr.String())
				if errMsg != "" {
					return nil, fmt.Errorf("checkupdates failed: %w (%s)", runErr, errMsg)
				}
				return nil, fmt.Errorf("checkupdates failed: %w", runErr)
			}
		}
		return parsePacmanCheckupdates(stdout.String())
	}

	// Fallback to pacman -Qu
	exe, err := p.Path()
	if err != nil {
		return nil, err
	}

	runErr := runner.Run(ctx, exe, []string{"-Qu"}, nil, &stdout, &stderr)
	if runErr != nil {
		exitCode := -1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if strings.Contains(runErr.Error(), "2") {
			exitCode = 2
		} else if strings.Contains(runErr.Error(), "1") {
			exitCode = 1
		}
		// exit code 1 (pacman -Qu) or 2 (checkupdates) means up to date
		if exitCode != 1 && exitCode != 2 {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return nil, fmt.Errorf("pacman -Qu failed: %w (%s)", runErr, errMsg)
			}
			return nil, fmt.Errorf("pacman -Qu failed: %w", runErr)
		}
	}

	return parsePacmanCheckupdates(stdout.String())
}

var pacmanLineRegex = regexp.MustCompile(`^([a-z0-9][a-z0-9@._+\-]*)\s+([^\s]+)\s+->\s+([^\s]+)`)

func parsePacmanCheckupdates(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "warning:") || strings.HasPrefix(line, "error:") {
			continue
		}
		m := pacmanLineRegex.FindStringSubmatch(line)
		if len(m) == 4 {
			id := m[1]
			curVer := m[2]
			availVer := m[3]

			if pm.ValidatePacmanID(id) != nil {
				continue
			}

			if version.IsUpgrade("pacman", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             id,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           "pacman",
					ManagerName:      "pacman",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
