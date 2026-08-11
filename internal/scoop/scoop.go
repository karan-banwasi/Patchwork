package scoop

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

// ScoopManager implements the pm.Manager interface for Scoop package manager.
type ScoopManager struct{}

// NewScoopManager creates a new instance of ScoopManager.
func NewScoopManager() *ScoopManager {
	return &ScoopManager{}
}

func (s *ScoopManager) Name() string {
	return "scoop"
}

func (s *ScoopManager) IsAvailable(ctx context.Context) bool {
	_, err := scoopPath()
	return err == nil
}

func (s *ScoopManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return GetAvailableUpdates(ctx)
}

func (s *ScoopManager) UpgradePackage(ctx context.Context, id string) error {
	return UpgradePackage(ctx, id)
}

func (s *ScoopManager) UpgradeAll(ctx context.Context) error {
	return UpgradeAll(ctx)
}

// scoopPath resolves the path to scoop CLI executable.
func scoopPath() (string, error) {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		shimCmd := filepath.Join(userProfile, "scoop", "shims", "scoop.cmd")
		if _, err := os.Stat(shimCmd); err == nil {
			return shimCmd, nil
		}
		shimExe := filepath.Join(userProfile, "scoop", "shims", "scoop.exe")
		if _, err := os.Stat(shimExe); err == nil {
			return shimExe, nil
		}
	}
	if p, err := exec.LookPath("scoop.cmd"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("scoop"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("scoop executable not found")
}

// GetAvailableUpdates runs `scoop status` to retrieve outdated packages.
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := scoopPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, exe, "status")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	if strings.Contains(outputStr, "Everything is up to date") ||
		strings.Contains(outputStr, "Scoop is up to date and no installed packages have updates available") {
		return []pm.PackageUpdate{}, nil
	}

	updates, parseErr := parseScoopOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	if err != nil && len(updates) == 0 {
		errMsg := fmt.Sprintf("scoop status failed: %v", err)
		if stderrStr != "" {
			errMsg += fmt.Sprintf(" (stderr: %s)", stderrStr)
		} else if strings.TrimSpace(outputStr) != "" {
			errMsg += fmt.Sprintf(" (stdout: %s)", strings.TrimSpace(outputStr))
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return updates, nil
}

// parseScoopOutput parses the output from `scoop status`.
func parseScoopOutput(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	cleanOutput := ansiEscape.ReplaceAllString(output, "")
	lines := strings.Split(cleanOutput, "\n")

	headerIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "Name") && (strings.Contains(trimmed, "Installed") || strings.Contains(trimmed, "Version")) && strings.Contains(trimmed, "Latest") {
			headerIdx = i
			break
		}
	}

	// Tabular parsing mode
	if headerIdx != -1 {
		headerLine := lines[headerIdx]
		nameIdx := strings.Index(headerLine, "Name")
		instIdx := strings.Index(headerLine, "Installed")
		if instIdx == -1 {
			instIdx = strings.Index(headerLine, "Version")
		}
		latestIdx := strings.Index(headerLine, "Latest")

		if nameIdx != -1 && instIdx != -1 && latestIdx != -1 && nameIdx < instIdx && instIdx < latestIdx {
			// Find optional 4th column (e.g., Missing dependencies or Bucket)
			missingIdx := -1
			for _, colName := range []string{"Missing", "Dependencies", "Bucket"} {
				idx := strings.Index(headerLine, colName)
				if idx > latestIdx {
					missingIdx = idx
					break
				}
			}

			startRow := headerIdx + 1
			if startRow < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[startRow]), "----") {
				startRow++
			}

			for i := startRow; i < len(lines); i++ {
				line := strings.TrimRight(lines[i], "\r\n ")
				if line == "" {
					continue
				}
				runes := []rune(line)
				if len(runes) <= latestIdx {
					continue
				}

				name := strings.TrimSpace(string(runes[nameIdx:instIdx]))
				inst := ""
				if len(runes) > latestIdx {
					inst = strings.TrimSpace(string(runes[instIdx:latestIdx]))
				} else {
					inst = strings.TrimSpace(string(runes[instIdx:]))
				}

				latest := ""
				if missingIdx != -1 && len(runes) > missingIdx {
					latest = strings.TrimSpace(string(runes[latestIdx:missingIdx]))
				} else if len(runes) > latestIdx {
					latest = strings.TrimSpace(string(runes[latestIdx:]))
				}

				if name != "" && !strings.HasPrefix(name, "-") && !strings.Contains(name, " ") {
					updates = append(updates, pm.PackageUpdate{
						Name:             name,
						Id:               name,
						Version:          inst,
						AvailableVersion: latest,
						Source:           "scoop",
						ManagerName:      "scoop",
					})
				}
			}
			return updates, nil
		}
	}

	// Fallback line-by-line regex parsing (e.g., "  git: 2.40.0 -> 2.41.0")
	lineRegex := regexp.MustCompile(`^\s*([^\s:]+):\s*([^\s]+)\s*->\s*([^\s]+)`)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		matches := lineRegex.FindStringSubmatch(trimmed)
		if len(matches) == 4 {
			updates = append(updates, pm.PackageUpdate{
				Name:             matches[1],
				Id:               matches[1],
				Version:          matches[2],
				AvailableVersion: matches[3],
				Source:           "scoop",
				ManagerName:      "scoop",
			})
		}
	}

	return updates, nil
}
