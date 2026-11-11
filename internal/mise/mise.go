package mise

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
	"github.com/karan-banwasi/patchwork/internal/version"
)

var (
	_ pm.Manager        = (*MiseManager)(nil)
	_ pm.Locator        = (*MiseManager)(nil)
	_ pm.Cleaner        = (*MiseManager)(nil)
	_ pm.BulkUpgrader   = (*MiseManager)(nil)
	_ pm.IOConfigurable = (*MiseManager)(nil)
)

// MiseManager implements the pm.Manager interface for mise-en-place runtime tools.
type MiseManager struct {
	runner  pm.Runner
	exePath string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewMiseManager() *MiseManager {
	return &MiseManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (m *MiseManager) Name() string {
	return "mise"
}

func (m *MiseManager) Aliases() []string {
	return []string{"rtx"}
}

func (m *MiseManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	m.stdin = stdin
	if stdout != nil {
		m.stdout = stdout
	} else {
		m.stdout = io.Discard
	}
	if stderr != nil {
		m.stderr = stderr
	} else {
		m.stderr = io.Discard
	}
}

func (m *MiseManager) ValidateID(id string) error {
	return pm.ValidateMiseID(id)
}

func (m *MiseManager) Path() (string, error) {
	if m.exePath != "" {
		return m.exePath, nil
	}
	for _, name := range []string{"mise.exe", "mise"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("mise executable not found")
}

func (m *MiseManager) Version(ctx context.Context) (string, error) {
	exe, err := m.Path()
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

func (m *MiseManager) IsAvailable(ctx context.Context) bool {
	_, err := m.Path()
	return err == nil
}

func (m *MiseManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := m.Path()
	if err != nil {
		return nil
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	fmt.Fprintln(out, "--- Pruning unused mise runtimes ---")
	return runner.Run(ctx, exe, []string{"prune"}, m.stdin, out, errOut)
}

func (m *MiseManager) UpgradePackage(ctx context.Context, id string) error {
	return m.UpgradePackages(ctx, []string{id})
}

func (m *MiseManager) UpgradePackages(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := m.ValidateID(id); err != nil {
			return err
		}
	}
	exe, err := m.Path()
	if err != nil {
		return err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	stdout := m.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := m.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := append([]string{"upgrade"}, ids...)
	if err := runner.Run(ctx, exe, args, m.stdin, stdout, stderr); err != nil {
		return fmt.Errorf("mise upgrade failed for %s: %w", strings.Join(ids, ", "), err)
	}
	return nil
}

func (m *MiseManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := m.Path()
	if err != nil {
		return nil, err
	}
	runner := m.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	if err := runner.Run(ctx, exe, []string{"outdated", "--json"}, nil, &stdout, &stderr); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("mise outdated failed: %w (%s)", err, errMsg)
		}
		return nil, fmt.Errorf("mise outdated failed: %w", err)
	}

	return parseMiseOutdatedJSON(stdout.String())
}

type miseOutdatedEntry struct {
	Name    string `json:"name"`
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

func parseMiseOutdatedJSON(output string) ([]pm.PackageUpdate, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "[]" || output == "{}" {
		return []pm.PackageUpdate{}, nil
	}

	var rawEntries []miseOutdatedEntry
	if err := json.Unmarshal([]byte(output), &rawEntries); err != nil {
		// Attempt parsing as map if mise returns an object
		var mapEntries map[string]miseOutdatedEntry
		if mapErr := json.Unmarshal([]byte(output), &mapEntries); mapErr == nil {
			for k, v := range mapEntries {
				if v.Name == "" {
					v.Name = k
				}
				rawEntries = append(rawEntries, v)
			}
		} else {
			return nil, fmt.Errorf("failed to parse mise outdated JSON: %w", err)
		}
	}

	var updates []pm.PackageUpdate
	for _, entry := range rawEntries {
		id := strings.TrimSpace(entry.Name)
		curVer := strings.TrimSpace(entry.Current)
		availVer := strings.TrimSpace(entry.Latest)

		if pm.ValidateMiseID(id) != nil {
			continue
		}

		if curVer != "" && availVer != "" && version.IsUpgrade("mise", curVer, availVer) {
			updates = append(updates, pm.PackageUpdate{
				Name:             id,
				Id:               id,
				Version:          curVer,
				AvailableVersion: availVer,
				Source:           "mise",
				ManagerName:      "mise",
			})
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		return strings.ToLower(updates[i].Name) < strings.ToLower(updates[j].Name)
	})

	return updates, nil
}
