package asdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*AsdfManager)(nil)
	_ pm.Locator        = (*AsdfManager)(nil)
	_ pm.Refresher      = (*AsdfManager)(nil)
	_ pm.IOConfigurable = (*AsdfManager)(nil)
)

// AsdfManager implements the pm.Manager interface for asdf tool version manager.
type AsdfManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewAsdfManager() *AsdfManager {
	return &AsdfManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (a *AsdfManager) Name() string {
	return "asdf"
}

func (a *AsdfManager) Aliases() []string {
	return []string{}
}

func (a *AsdfManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
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

func (a *AsdfManager) ValidateID(id string) error {
	return pm.ValidateAsdfID(id)
}

func (a *AsdfManager) Path() (string, error) {
	if a.exePath != "" {
		return a.exePath, nil
	}
	return asdfPath()
}

func asdfPath() (string, error) {
	for _, name := range []string{"asdf.bat", "asdf.cmd", "asdf"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		p := filepath.Join(home, ".asdf", "bin", "asdf")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("asdf executable not found")
}

func (a *AsdfManager) Version(ctx context.Context) (string, error) {
	exe, err := a.Path()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "version").Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}

func (a *AsdfManager) IsAvailable(ctx context.Context) bool {
	_, err := a.Path()
	return err == nil
}

func (a *AsdfManager) Refresh(ctx context.Context) error {
	exe, err := a.Path()
	if err != nil {
		return err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"plugin", "update", "--all"}, a.stdin, a.stdout, a.stderr)
}

func (a *AsdfManager) UpgradePackage(ctx context.Context, id string) error {
	if err := a.ValidateID(id); err != nil {
		return err
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

	if err := runner.Run(ctx, exe, []string{"install", id, "latest"}, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("asdf install latest failed for %s: %w", id, err)
	}
	return nil
}

func parseAsdfCurrent(output string) ([]pm.PackageUpdate, error) {
	var pkgs []pm.PackageUpdate
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "No versions") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		id := fields[0]
		ver := fields[1]

		if pm.ValidateAsdfID(id) != nil {
			continue
		}

		pkgs = append(pkgs, pm.PackageUpdate{
			Name:        id,
			Id:          id,
			Version:     ver,
			Source:      "asdf",
			ManagerName: "asdf",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].Name) < strings.ToLower(pkgs[j].Name)
	})

	return pkgs, nil
}

func (a *AsdfManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := a.Path()
	if err != nil {
		return nil, err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"current"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("asdf current failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("asdf current failed: %w", err)
	}

	installed, err := parseAsdfCurrent(stdout.String())
	if err != nil {
		return nil, err
	}

	var updates []pm.PackageUpdate
	for _, pkg := range installed {
		var outBuf bytes.Buffer
		if err := runner.Run(ctx, exe, []string{"latest", pkg.Id}, nil, &outBuf, io.Discard); err == nil {
			latest := strings.TrimSpace(outBuf.String())
			if latest != "" && version.IsUpgrade("asdf", pkg.Version, latest) {
				pkg.AvailableVersion = latest
				updates = append(updates, pkg)
			}
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
