package apt

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
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager         = (*AptManager)(nil)
	_ pm.Locator         = (*AptManager)(nil)
	_ pm.Cleaner         = (*AptManager)(nil)
	_ pm.Refresher       = (*AptManager)(nil)
	_ pm.BulkUpgrader    = (*AptManager)(nil)
	_ pm.IOConfigurable  = (*AptManager)(nil)
	_ pm.TimeoutProvider = (*AptManager)(nil)
)

// AptManager implements the pm.Manager interface for Debian/Ubuntu APT packages.
type AptManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewAptManager() *AptManager {
	return &AptManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (a *AptManager) Name() string {
	return "apt"
}

func (a *AptManager) Aliases() []string {
	return []string{"apt-get", "debian", "ubuntu"}
}

func (a *AptManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	a.stdin = stdin
	if stdout != nil {
		a.stdout = stdout
	} else {
		a.stdout = io.Discard
	}
	if stderr != nil {
		a.stderr = stderr
	} else {
		a.stderr = io.Discard
	}
}

func (a *AptManager) FetchTimeout() time.Duration {
	return 2 * time.Minute
}

func (a *AptManager) ValidateID(id string) error {
	return pm.ValidateAptID(id)
}

func (a *AptManager) Path() (string, error) {
	if a.exePath != "" {
		return a.exePath, nil
	}
	return aptPath()
}

func (a *AptManager) Version(ctx context.Context) (string, error) {
	exe, err := a.Path()
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

func (a *AptManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := a.Path()
	return err == nil
}

func aptPath() (string, error) {
	for _, name := range []string{"apt-get", "apt"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/apt-get", "/usr/bin/apt"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("apt executable not found")
}

func wrapSudo(cmd string, args []string) (string, []string) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil {
			return sudoPath, append([]string{cmd}, args...)
		}
	}
	return cmd, args
}

func (a *AptManager) Refresh(ctx context.Context) error {
	exe, err := a.Path()
	if err != nil {
		return err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	runExe, runArgs := wrapSudo(exe, []string{"update"})
	return runner.Run(ctx, runExe, runArgs, a.stdin, a.stdout, a.stderr)
}

func (a *AptManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := a.Path()
	if err != nil {
		return nil
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up APT packages and cache ---")
	runExe, runArgs := wrapSudo(exe, []string{"autoremove", "-y"})
	if err := runner.Run(ctx, runExe, runArgs, a.stdin, out, errOut); err != nil {
		return err
	}
	runExe, runArgs = wrapSudo(exe, []string{"clean"})
	return runner.Run(ctx, runExe, runArgs, a.stdin, out, errOut)
}

func (a *AptManager) UpgradePackage(ctx context.Context, id string) error {
	return a.UpgradePackages(ctx, []string{id})
}

func (a *AptManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := a.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := a.Path()
	if err != nil {
		return err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := a.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := a.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"install", "--only-upgrade", "-y"}, ids...)
	runExe, runArgs := wrapSudo(exe, args)
	if err := runner.Run(ctx, runExe, runArgs, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("apt upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (a *AptManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := a.Path()
	if err != nil {
		return nil, err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	// Use "apt" for list --upgradable
	listExe := exe
	if strings.HasSuffix(listExe, "apt-get") {
		if p, err := exec.LookPath("apt"); err == nil {
			listExe = p
		}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, listExe, []string{"list", "--upgradable"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("apt list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("apt list failed: %w", err)
	}

	return parseAptList(stdout.String())
}

var aptLineRegex = regexp.MustCompile(`^([a-z0-9][a-z0-9+\-.:]*)\/[^\s]+\s+([^\s]+)\s+([^\s]+)\s+\[upgradable from:\s+([^\s\]]+)\]`)

func parseAptList(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Listing...") || strings.HasPrefix(line, "WARNING:") {
			continue
		}
		m := aptLineRegex.FindStringSubmatch(line)
		if len(m) == 5 {
			id := m[1]
			availVer := m[2]
			curVer := m[4]
			if pm.ValidateAptID(id) == nil && version.IsUpgrade("apt", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             id,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           "apt",
					ManagerName:      "apt",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
