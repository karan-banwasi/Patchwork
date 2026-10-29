package choco

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHFABCDJsuhl?]`)

// ChocoManager implements the pm.Manager interface for Chocolatey package manager.
type ChocoManager struct {
	runner pm.Runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewChocoManager creates a new instance of ChocoManager.
func NewChocoManager() *ChocoManager {
	return &ChocoManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (c *ChocoManager) Name() string {
	return "choco"
}

func (c *ChocoManager) Aliases() []string {
	return []string{"chocolatey"}
}

func (c *ChocoManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	c.stdin = stdin
	if stdout != nil {
		c.stdout = stdout
	} else {
		c.stdout = io.Discard
	}
	if stderr != nil {
		c.stderr = stderr
	} else {
		c.stderr = io.Discard
	}
}

func (c *ChocoManager) ValidateID(id string) error {
	return pm.ValidateChocoID(id)
}

// Path resolves the path to the Chocolatey executable.
func (c *ChocoManager) Path() (string, error) {
	return chocoPath()
}

// Version returns the installed Chocolatey version string.
func (c *ChocoManager) Version(ctx context.Context) (string, error) {
	exe, err := chocoPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *ChocoManager) IsAvailable(ctx context.Context) bool {
	_, err := chocoPath()
	return err == nil
}

// chocoPath resolves the path to Chocolatey executable for non-elevated operations.
func chocoPath() (string, error) {
	if p, err := trustedChocoPath(); err == nil {
		return p, nil
	}
	programData := os.Getenv("ProgramData")
	if programData != "" {
		chocoExe := filepath.Join(programData, "chocolatey", "bin", "choco.exe")
		if _, err := os.Stat(chocoExe); err == nil {
			return chocoExe, nil
		}
	}
	if p, err := exec.LookPath("choco.exe"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("choco"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("choco executable not found")
}

// GetAvailableUpdates runs `choco outdated --limit-output` to fetch available updates.
func (c *ChocoManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := chocoPath()
	if err != nil {
		return nil, err
	}

	runner := c.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"outdated", "--limit-output"}, nil, &stdout, &stderr)
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	updates, parseErr := parseChocoOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	if err != nil && len(updates) == 0 {
		errMsg := fmt.Sprintf("choco outdated failed: %v", err)
		if stderrStr != "" {
			errMsg += fmt.Sprintf(" (stderr: %s)", stderrStr)
		} else if strings.TrimSpace(outputStr) != "" {
			errMsg += fmt.Sprintf(" (stdout: %s)", strings.TrimSpace(outputStr))
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return updates, nil
}

// GetAvailableUpdates is a package-level helper for convenience.
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return NewChocoManager().GetAvailableUpdates(ctx)
}

// parseChocoOutput parses the output of `choco outdated`.
func parseChocoOutput(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	cleanOutput := ansiEscape.ReplaceAllString(output, "")
	lines := strings.Split(cleanOutput, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Chocolatey v") || strings.HasPrefix(trimmed, "Output format:") {
			continue
		}
		if strings.Contains(trimmed, "has determined") || strings.Contains(trimmed, "packages are outdated") {
			continue
		}

		// Pipe-delimited format (name|currentVersion|availableVersion|pinned)
		if strings.Contains(trimmed, "|") {
			parts := strings.Split(trimmed, "|")
			if len(parts) >= 3 {
				if len(parts) >= 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "true") {
					continue
				}
				name := strings.TrimSpace(parts[0])
				current := strings.TrimSpace(parts[1])
				available := strings.TrimSpace(parts[2])

				if name != "" && current != "" && available != "" && pm.ValidateChocoID(name) == nil {
					updates = append(updates, pm.PackageUpdate{
						Name:             name,
						Id:               name,
						Version:          current,
						AvailableVersion: available,
						Source:           "chocolatey",
						ManagerName:      "choco",
					})
				}
			}
			continue
		}

		// Fallback for space-separated tabular format (e.g., "git.install 2.40.0 2.41.0 false")
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && !strings.HasPrefix(fields[0], "-") && !strings.EqualFold(fields[0], "Title") {
			if len(fields) >= 4 && strings.EqualFold(fields[3], "true") {
				continue
			}
			if pm.ValidateChocoID(fields[0]) == nil {
				updates = append(updates, pm.PackageUpdate{
					Name:             fields[0],
					Id:               fields[0],
					Version:          fields[1],
					AvailableVersion: fields[2],
					Source:           "chocolatey",
					ManagerName:      "choco",
				})
			}
		}
	}

	return updates, nil
}
