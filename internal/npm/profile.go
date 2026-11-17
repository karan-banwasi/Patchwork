package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*NpmManager)(nil)
	_ pm.Installer = (*NpmManager)(nil)
)

func isMockRunner(r pm.Runner) bool {
	if r == nil {
		return false
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return !isExec && !isExecPtr
}

func (n *NpmManager) resolveExe() (string, error) {
	exe, err := npmPath()
	if err != nil {
		if isMockRunner(n.runner) {
			return "npm", nil
		}
		return "", err
	}
	return exe, nil
}

type npmListJSON struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// ListInstalled runs `npm list -g --depth=0 --json` and parses globally installed npm packages.
func (n *NpmManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := n.resolveExe()
	if err != nil {
		return nil, err
	}
	runner := n.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	runErr := runner.Run(ctx, exe, []string{"list", "-g", "--depth=0", "--json"}, nil, &stdout, &stderr)
	outStr := strings.TrimSpace(stdout.String())
	if runErr != nil && outStr == "" {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("npm list failed: %w (%s)", runErr, stderrStr)
		}
		return nil, fmt.Errorf("npm list failed: %w", runErr)
	}

	return parseNpmListJSON(outStr)
}

func parseNpmListJSON(output string) ([]pm.InstalledPackage, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "{}" {
		return []pm.InstalledPackage{}, nil
	}

	var parsed npmListJSON
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse npm list JSON: %w", err)
	}

	var pkgs []pm.InstalledPackage
	for name, entry := range parsed.Dependencies {
		if pm.ValidateNpmID(name) != nil {
			continue
		}
		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        name,
			ID:          name,
			Version:     strings.TrimSpace(entry.Version),
			ManagerName: "npm",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a global npm package (optionally specifying version via `id@version`).
func (n *NpmManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := n.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := n.resolveExe()
	if err != nil {
		return err
	}
	runner := n.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := n.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := n.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	target := id
	if version != "" {
		target = id + "@" + version
	}
	if err := runner.Run(ctx, exe, []string{"install", "-g", target}, n.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("npm install failed for %s: %w", id, err)
	}
	return nil
}
