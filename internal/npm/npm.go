package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// NpmManager implements the pm.Manager interface for global npm packages.
type NpmManager struct{}

func NewNpmManager() *NpmManager {
	return &NpmManager{}
}

func (n *NpmManager) Name() string {
	return "npm"
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

	cmd := exec.CommandContext(ctx, exe, "outdated", "-g", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// npm outdated exits with code 1 when outdated packages exist
	_ = cmd.Run()
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
	exe, err := npmPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "install", "-g", id+"@latest")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm upgrade failed for %s: %w", id, err)
	}
	return nil
}

func (n *NpmManager) UpgradeAll(ctx context.Context) error {
	updates, err := n.GetAvailableUpdates(ctx)
	if err != nil {
		return err
	}
	for _, u := range updates {
		if err := n.UpgradePackage(ctx, u.Id); err != nil {
			return err
		}
	}
	return nil
}
