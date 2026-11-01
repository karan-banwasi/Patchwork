package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// NpmManager implements the pm.Manager interface for global npm packages.
type NpmManager struct {
	runner pm.Runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func NewNpmManager() *NpmManager {
	return &NpmManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (n *NpmManager) Name() string {
	return "npm"
}

func (n *NpmManager) Aliases() []string {
	return nil
}

func (n *NpmManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	n.stdin = stdin
	if stdout != nil {
		n.stdout = stdout
	} else {
		n.stdout = io.Discard
	}
	if stderr != nil {
		n.stderr = stderr
	} else {
		n.stderr = io.Discard
	}
}

func (n *NpmManager) ValidateID(id string) error {
	return pm.ValidateNpmID(id)
}

func (n *NpmManager) Path() (string, error) {
	return npmPath()
}

func (n *NpmManager) Version(ctx context.Context) (string, error) {
	exe, err := npmPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (n *NpmManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := npmPath()
	if err != nil {
		return nil
	}
	fmt.Fprintln(out, "--- Cleaning up npm cache ---")
	runner := n.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"cache", "clean", "--force"}, nil, out, errOut)
}

func (n *NpmManager) IsAvailable(ctx context.Context) bool {
	_, err := npmPath()
	return err == nil
}

func npmPath() (string, error) {
	for _, name := range []string{"npm.cmd", "npm.exe", "npm"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("npm executable not found")
}

type npmOutdatedEntry struct {
	Current string `json:"current"`
	Wanted  string `json:"wanted"`
	Latest  string `json:"latest"`
}

func (n *NpmManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := npmPath()
	if err != nil {
		return nil, err
	}

	runner := n.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	// npm outdated exits with code 1 when outdated packages exist
	_ = runner.Run(ctx, exe, []string{"outdated", "-g", "--json"}, nil, &stdout, &stderr)
	outputStr := strings.TrimSpace(stdout.String())
	if outputStr == "" || outputStr == "{}" {
		return []pm.PackageUpdate{}, nil
	}

	return parseNpmOutdatedJSON(outputStr)
}

func parseNpmOutdatedJSON(output string) ([]pm.PackageUpdate, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, nil
	}
	if _, isErr := raw["error"]; isErr {
		return nil, nil
	}

	var updates []pm.PackageUpdate
	for pkgName, msg := range raw {
		if pm.ValidateNpmID(pkgName) != nil {
			continue
		}
		var entry npmOutdatedEntry
		if err := json.Unmarshal(msg, &entry); err != nil {
			continue
		}
		avail := entry.Latest
		if avail == "" {
			avail = entry.Wanted
		}
		if entry.Current != "" && avail != "" && entry.Current != avail {
			updates = append(updates, pm.PackageUpdate{
				Name:             pkgName,
				Id:               pkgName,
				Version:          entry.Current,
				AvailableVersion: avail,
				Source:           "npm",
				ManagerName:      "npm",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].Name < updates[j].Name
	})
	return updates, nil
}

func (n *NpmManager) UpgradePackage(ctx context.Context, id string) error {
	if err := n.ValidateID(id); err != nil {
		return err
	}
	exe, err := npmPath()
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
	if err := runner.Run(ctx, exe, []string{"install", "-g", "--", id + "@latest"}, n.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("npm upgrade failed for %s: %w", id, err)
	}
	return nil
}
