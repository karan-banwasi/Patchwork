package dnf

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
	_ pm.Manager         = (*DnfManager)(nil)
	_ pm.Locator         = (*DnfManager)(nil)
	_ pm.Cleaner         = (*DnfManager)(nil)
	_ pm.Refresher       = (*DnfManager)(nil)
	_ pm.BulkUpgrader    = (*DnfManager)(nil)
	_ pm.IOConfigurable  = (*DnfManager)(nil)
	_ pm.TimeoutProvider = (*DnfManager)(nil)
)

// DnfManager implements the pm.Manager interface for Fedora/RHEL DNF packages.
type DnfManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewDnfManager() *DnfManager {
	return &DnfManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (d *DnfManager) Name() string {
	return "dnf"
}

func (d *DnfManager) Aliases() []string {
	return []string{"yum", "fedora", "rhel"}
}

func (d *DnfManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	d.stdin = stdin
	if stdout != nil {
		d.stdout = stdout
	} else {
		d.stdout = io.Discard
	}
	if stderr != nil {
		d.stderr = stderr
	} else {
		d.stderr = io.Discard
	}
}

func (d *DnfManager) FetchTimeout() time.Duration {
	return 2 * time.Minute
}

func (d *DnfManager) ValidateID(id string) error {
	return pm.ValidateDnfID(id)
}

func (d *DnfManager) Path() (string, error) {
	if d.exePath != "" {
		return d.exePath, nil
	}
	return dnfPath()
}

func (d *DnfManager) Version(ctx context.Context) (string, error) {
	exe, err := d.Path()
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

func (d *DnfManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := d.Path()
	return err == nil
}

func dnfPath() (string, error) {
	for _, name := range []string{"dnf", "yum"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/dnf", "/usr/bin/yum"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("dnf executable not found")
}

func wrapSudo(cmd string, args []string) (string, []string) {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		if sudoPath, err := exec.LookPath("sudo"); err == nil {
			return sudoPath, append([]string{cmd}, args...)
		}
	}
	return cmd, args
}

func (d *DnfManager) Refresh(ctx context.Context) error {
	exe, err := d.Path()
	if err != nil {
		return err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	runExe, runArgs := wrapSudo(exe, []string{"makecache"})
	return runner.Run(ctx, runExe, runArgs, d.stdin, d.stdout, d.stderr)
}

func (d *DnfManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := d.Path()
	if err != nil {
		return nil
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up DNF cache and packages ---")
	runExe, runArgs := wrapSudo(exe, []string{"clean", "all"})
	if err := runner.Run(ctx, runExe, runArgs, d.stdin, out, errOut); err != nil {
		return err
	}
	runExe, runArgs = wrapSudo(exe, []string{"autoremove", "-y"})
	return runner.Run(ctx, runExe, runArgs, d.stdin, out, errOut)
}

func (d *DnfManager) UpgradePackage(ctx context.Context, id string) error {
	return d.UpgradePackages(ctx, []string{id})
}

func (d *DnfManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := d.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := d.Path()
	if err != nil {
		return err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := d.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := d.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"upgrade", "-y"}, ids...)
	runExe, runArgs := wrapSudo(exe, args)
	if err := runner.Run(ctx, runExe, runArgs, d.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("dnf upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (d *DnfManager) queryInstalledRPMs(ctx context.Context) map[string]string {
	rpmExe, err := exec.LookPath("rpm")
	if err != nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, rpmExe, "-qa", "--qf", "%{NAME} %{VERSION}-%{RELEASE}\n").Output()
	if err != nil {
		return nil
	}
	installed := make(map[string]string)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			installed[fields[0]] = fields[1]
		}
	}
	return installed
}

func (d *DnfManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := d.Path()
	if err != nil {
		return nil, err
	}
	runner := d.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"check-update", "--quiet"}, nil, &stdout, &stderr)
	// dnf check-update returns exit code 100 if updates are available, 0 if no updates
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if strings.Contains(err.Error(), "100") {
			exitCode = 100
		}
		if exitCode != 100 {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return nil, fmt.Errorf("dnf check-update failed: %w (%s)", err, errMsg)
			}
			return nil, fmt.Errorf("dnf check-update failed: %w", err)
		}
	}

	installedMap := d.queryInstalledRPMs(ctx)
	return parseDnfCheckUpdate(stdout.String(), installedMap)
}

var dnfLineRegex = regexp.MustCompile(`^([A-Za-z0-9._\-+]+)\.([A-Za-z0-9_]+)\s+([^\s]+)\s+([^\s]+)`)

func parseDnfCheckUpdate(output string, installedMap map[string]string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Security:") || strings.HasPrefix(line, "Obsoleting") {
			continue
		}
		m := dnfLineRegex.FindStringSubmatch(line)
		if len(m) == 5 {
			id := m[1]
			availVer := m[3]
			repo := m[4]

			if pm.ValidateDnfID(id) != nil {
				continue
			}

			curVer := "unknown"
			if installedMap != nil {
				if v, ok := installedMap[id]; ok {
					curVer = v
				}
			}

			if curVer == "unknown" || version.IsUpgrade("dnf", curVer, availVer) {
				updates = append(updates, pm.PackageUpdate{
					Name:             id,
					Id:               id,
					Version:          curVer,
					AvailableVersion: availVer,
					Source:           repo,
					ManagerName:      "dnf",
				})
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
