package gem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*GemManager)(nil)
	_ pm.Installer = (*GemManager)(nil)
)

var gemListRegex = regexp.MustCompile(`^([a-zA-Z0-9._\-]+)\s+\(([^)]+)\)`)

// ListInstalled runs `gem list --local` and parses locally installed RubyGems.
func (g *GemManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := g.Path()
	if err != nil {
		return nil, err
	}
	runner := g.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"list", "--local"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("gem list failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("gem list failed: %w", err)
	}

	return parseGemList(stdout.String()), nil
}

func parseGemList(output string) []pm.InstalledPackage {
	var pkgs []pm.InstalledPackage
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "***") {
			continue
		}
		m := gemListRegex.FindStringSubmatch(line)
		if len(m) == 3 {
			id := m[1]
			if pm.ValidateGemID(id) != nil {
				continue
			}
			verPart := strings.Split(m[2], ",")[0]
			verPart = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(verPart), "default:"))
			pkgs = append(pkgs, pm.InstalledPackage{
				Name:        id,
				ID:          id,
				Version:     verPart,
				ManagerName: "gem",
			})
		}
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs
}

// InstallPackage installs a Ruby gem via `gem install <id>` (with optional `-v <version>`).
func (g *GemManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := g.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := g.Path()
	if err != nil {
		return err
	}
	runner := g.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := g.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := g.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := []string{"install", id}
	if version != "" {
		args = append(args, "-v", version)
	}
	if err := runner.Run(ctx, exe, args, g.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("gem install failed for %s: %w", id, err)
	}
	return nil
}
