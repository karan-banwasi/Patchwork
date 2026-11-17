package apt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*AptManager)(nil)
	_ pm.Installer = (*AptManager)(nil)
)

var aptInstalledRegex = regexp.MustCompile(`^([a-z0-9][a-z0-9+\-.:]*)\/[^\s]+\s+([^\s]+)\s+`)

// ListInstalled runs `apt list --installed` and parses installed Debian/Ubuntu packages.
func (a *AptManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := a.Path()
	if err != nil {
		return nil, err
	}
	runner := a.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	listExe := exe
	if strings.HasSuffix(listExe, "apt-get") {
		if p, err := exec.LookPath("apt"); err == nil {
			listExe = p
		}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, listExe, []string{"list", "--installed"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("apt list --installed failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("apt list --installed failed: %w", err)
	}

	return parseAptInstalled(stdout.String()), nil
}

func parseAptInstalled(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "Listing...") || strings.HasPrefix(line, "WARNING:") {
			continue
		}
		m := aptInstalledRegex.FindStringSubmatch(line)
		if len(m) == 3 {
			id := m[1]
			ver := m[2]
			if pm.ValidateAptID(id) == nil {
				pkgs = append(pkgs, pm.InstalledPackage{
					Name:        id,
					ID:          id,
					Version:     ver,
					ManagerName: "apt",
				})
			}
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs an APT package (optionally pinned to a version via `id=version`).
func (a *AptManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := a.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
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

	target := id
	if version != "" {
		target = id + "=" + version
	}
	runExe, runArgs := wrapSudo(exe, []string{"install", "-y", target})
	if err := runner.Run(ctx, runExe, runArgs, a.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("apt install failed for %s: %w", id, err)
	}
	return nil
}
