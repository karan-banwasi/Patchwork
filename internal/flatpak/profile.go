package flatpak

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
	_ pm.Lister    = (*FlatpakManager)(nil)
	_ pm.Installer = (*FlatpakManager)(nil)
)

// ListInstalled runs `flatpak list --app --columns=application,name,version` and parses installed Flatpak apps.
func (f *FlatpakManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := f.Path()
	if err != nil {
		return nil, err
	}
	runner := f.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--app", "--columns=application,name,version"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("flatpak list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("flatpak list failed: %w", err)
	}

	return parseFlatpakInstalled(stdout.String()), nil
}

func parseFlatpakInstalled(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var appID, name, ver string
		if strings.Contains(line, "\t") {
			parts := strings.Split(line, "\t")
			if len(parts) >= 3 {
				appID = strings.TrimSpace(parts[0])
				name = strings.TrimSpace(parts[1])
				ver = strings.TrimSpace(parts[2])
			} else if len(parts) == 2 {
				appID = strings.TrimSpace(parts[0])
				name = appID
				ver = strings.TrimSpace(parts[1])
			}
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				appID = fields[0]
				name = fields[1]
				ver = fields[2]
			} else if len(fields) == 2 {
				appID = fields[0]
				name = appID
				ver = fields[1]
			}
		}

		if appID == "" || pm.ValidateFlatpakID(appID) != nil {
			continue
		}
		if name == "" {
			name = appID
		}
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        name,
			ID:          appID,
			Version:     ver,
			ManagerName: "flatpak",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a Flatpak application via `flatpak install -y <id>`.
func (f *FlatpakManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := f.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
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

	if err := runner.Run(ctx, exe, []string{"install", "-y", id}, f.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("flatpak install failed for %s: %w", id, err)
	}
	return nil
}
