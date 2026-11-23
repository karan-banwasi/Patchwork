package flatpak

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
	_ pm.Manager        = (*FlatpakManager)(nil)
	_ pm.Locator        = (*FlatpakManager)(nil)
	_ pm.Cleaner        = (*FlatpakManager)(nil)
	_ pm.BulkUpgrader   = (*FlatpakManager)(nil)
	_ pm.IOConfigurable = (*FlatpakManager)(nil)
)

// FlatpakManager implements the pm.Manager interface for Linux Flatpak applications.
type FlatpakManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewFlatpakManager() *FlatpakManager {
	return &FlatpakManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (f *FlatpakManager) Name() string {
	return "flatpak"
}

func (f *FlatpakManager) Aliases() []string {
	return []string{"flathub"}
}

func (f *FlatpakManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	f.stdin = stdin
	if stdout != nil {
		f.stdout = stdout
	} else {
		f.stdout = io.Discard
	}
	if stderr != nil {
		f.stderr = stderr
	} else {
		f.stderr = io.Discard
	}
}

func (f *FlatpakManager) ValidateID(id string) error {
	return pm.ValidateFlatpakID(id)
}

func (f *FlatpakManager) Path() (string, error) {
	if f.exePath != "" {
		return f.exePath, nil
	}
	return flatpakPath()
}

func (f *FlatpakManager) Version(ctx context.Context) (string, error) {
	exe, err := f.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimPrefix(strings.TrimSpace(lines[0]), "Flatpak "), nil
	}
	return "unknown", nil
}

func (f *FlatpakManager) IsAvailable(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := f.Path()
	return err == nil
}

func flatpakPath() (string, error) {
	for _, name := range []string{"flatpak"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{"/usr/bin/flatpak"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("flatpak executable not found")
}

func (f *FlatpakManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := f.Path()
	if err != nil {
		return nil
	}
	runner := f.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Cleaning up unused Flatpak runtimes ---")
	return runner.Run(ctx, exe, []string{"uninstall", "--unused", "-y"}, f.stdin, out, errOut)
}

func (f *FlatpakManager) UpgradePackage(ctx context.Context, id string) error {
	return f.UpgradePackages(ctx, []string{id})
}

func (f *FlatpakManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := f.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := f.Path()
	if err != nil {
		return err
	}
	runner := f.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := f.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := f.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"update", "-y"}, ids...)
	if err := runner.Run(ctx, exe, args, f.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("flatpak update failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (f *FlatpakManager) queryInstalledApps(ctx context.Context, exe string, runner pm.Runner) map[string]string {
	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--app", "--columns=application,version"}, nil, &stdout, &stderr); err != nil {
		return nil
	}
	installed := make(map[string]string)
	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) >= 2 {
			installed[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		} else {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) >= 2 {
				installed[fields[0]] = fields[1]
			}
		}
	}
	return installed
}

func (f *FlatpakManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := f.Path()
	if err != nil {
		return nil, err
	}
	runner := f.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"remote-ls", "--updates", "--columns=application,name,version"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("flatpak remote-ls failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("flatpak remote-ls failed: %w", err)
	}

	installedMap := f.queryInstalledApps(ctx, exe, runner)
	return parseFlatpakUpdates(stdout.String(), installedMap)
}

func parseFlatpakUpdates(output string, installedMap map[string]string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		var appID, name, availVer string
		if strings.Contains(line, "\t") {
			parts := strings.Split(line, "\t")
			if len(parts) >= 3 {
				appID = strings.TrimSpace(parts[0])
				name = strings.TrimSpace(parts[1])
				availVer = strings.TrimSpace(parts[2])
			} else if len(parts) == 2 {
				appID = strings.TrimSpace(parts[0])
				name = appID
				availVer = strings.TrimSpace(parts[1])
			}
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				appID = fields[0]
				name = fields[1]
				availVer = fields[2]
			} else if len(fields) == 2 {
				appID = fields[0]
				name = fields[0]
				availVer = fields[1]
			}
		}

		if appID == "" || pm.ValidateFlatpakID(appID) != nil {
			continue
		}

		curVer := "unknown"
		if installedMap != nil {
			if v, ok := installedMap[appID]; ok {
				curVer = v
			}
		}

		if curVer == "unknown" || version.IsUpgrade("flatpak", curVer, availVer) {
			updates = append(updates, pm.PackageUpdate{
				Name:             name,
				Id:               appID,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "flathub",
				ManagerName:      "flatpak",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
