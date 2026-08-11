package choco

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHFABCDJsuhl?]`)

// ChocoManager implements the pm.Manager interface for Chocolatey package manager.
type ChocoManager struct{}

// NewChocoManager creates a new instance of ChocoManager.
func NewChocoManager() *ChocoManager {
	return &ChocoManager{}
}

func (c *ChocoManager) Name() string {
	return "choco"
}

func (c *ChocoManager) IsAvailable(ctx context.Context) bool {
	_, err := chocoPath()
	return err == nil
}

func (c *ChocoManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return GetAvailableUpdates(ctx)
}

func (c *ChocoManager) UpgradePackage(ctx context.Context, id string) error {
	return UpgradePackage(ctx, id)
}

func (c *ChocoManager) UpgradeAll(ctx context.Context) error {
	return UpgradeAll(ctx)
}

// chocoPath resolves the path to Chocolatey executable.
func chocoPath() (string, error) {
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
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	if !isElevated() {
		return nil, fmt.Errorf("Chocolatey is installed, but querying/upgrading packages requires Administrator privileges. Please rerun terminal as Administrator to include Chocolatey updates")
	}

	exe, err := chocoPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, exe, "outdated", "--limit-output")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
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
				name := strings.TrimSpace(parts[0])
				current := strings.TrimSpace(parts[1])
				available := strings.TrimSpace(parts[2])

				if name != "" && current != "" && available != "" {
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

	return updates, nil
}
