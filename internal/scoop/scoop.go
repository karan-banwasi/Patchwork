package scoop

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

// ScoopManager implements the pm.Manager interface for Scoop package manager.
type ScoopManager struct {
	runner pm.Runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewScoopManager creates a new instance of ScoopManager.
func NewScoopManager() *ScoopManager {
	return &ScoopManager{
		runner: pm.ExecRunner{},
		stdout: io.Discard,
		stderr: io.Discard,
	}
}

func (s *ScoopManager) Name() string {
	return "scoop"
}

func (s *ScoopManager) Aliases() []string {
	return nil
}

func (s *ScoopManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	s.stdin = stdin
	if stdout != nil {
		s.stdout = stdout
	} else {
		s.stdout = io.Discard
	}
	if stderr != nil {
		s.stderr = stderr
	} else {
		s.stderr = io.Discard
	}
}

func (s *ScoopManager) ValidateID(id string) error {
	return pm.ValidateScoopID(id)
}

// Path resolves the path to the scoop CLI executable.
func (s *ScoopManager) Path() (string, error) {
	return scoopPath()
}

// Version returns the installed status/version for Scoop.
func (s *ScoopManager) Version(ctx context.Context) (string, error) {
	if _, err := scoopPath(); err != nil {
		return "", err
	}
	return "installed", nil
}

// Cleanup removes outdated package versions and clears the Scoop download cache.
func (s *ScoopManager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	exe, err := scoopPath()
	if err != nil {
		return nil
	}
	fmt.Fprintln(out, "--- Cleaning up Scoop (old versions & download cache) ---")
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	_ = runner.Run(ctx, exe, []string{"cleanup", "*"}, nil, out, errOut)
	return runner.Run(ctx, exe, []string{"cache", "rm", "*"}, nil, out, errOut)
}

func (s *ScoopManager) IsAvailable(ctx context.Context) bool {
	_, err := scoopPath()
	return err == nil
}

// Refresh updates local Scoop buckets so subsequent `scoop status` calls compare against upstream manifests.
func (s *ScoopManager) Refresh(ctx context.Context) error {
	exe, err := scoopPath()
	if err != nil {
		return err
	}
	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}
	return runner.Run(ctx, exe, []string{"update"}, nil, io.Discard, io.Discard)
}

// scoopPath resolves the path to scoop CLI executable.
func scoopPath() (string, error) {
	// Check custom SCOOP and SCOOP_GLOBAL environment variables
	for _, envKey := range []string{"SCOOP", "SCOOP_GLOBAL"} {
		if envVal := os.Getenv(envKey); envVal != "" {
			for _, shim := range []string{"scoop.cmd", "scoop.exe"} {
				path := filepath.Join(envVal, "shims", shim)
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
			}
		}
	}

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
	for _, name := range []string{"scoop.cmd", "scoop.exe", "scoop"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("scoop executable not found")
}

// GetAvailableUpdates runs `scoop status` (read-only) to retrieve outdated packages.
func (s *ScoopManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := scoopPath()
	if err != nil {
		return nil, err
	}

	runner := s.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"status"}, nil, &stdout, &stderr)
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

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

// GetAvailableUpdates is a package-level helper for convenience.
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return NewScoopManager().GetAvailableUpdates(ctx)
}

// indexRunes returns the rune index of sub within runes, or -1 if not found.
func indexRunes(runes []rune, sub string) int {
	subRunes := []rune(sub)
	if len(subRunes) == 0 {
		return 0
	}
	for i := 0; i <= len(runes)-len(subRunes); i++ {
		match := true
		for j := 0; j < len(subRunes); j++ {
			if runes[i+j] != subRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
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
		headerRunes := []rune(lines[headerIdx])
		nameIdx := indexRunes(headerRunes, "Name")
		instIdx := indexRunes(headerRunes, "Installed")
		if instIdx == -1 {
			instIdx = indexRunes(headerRunes, "Version")
		}
		latestIdx := indexRunes(headerRunes, "Latest")

		if nameIdx != -1 && instIdx != -1 && latestIdx != -1 && nameIdx < instIdx && instIdx < latestIdx {
			// Find optional 4th column (e.g., Missing dependencies or Bucket)
			missingIdx := -1
			for _, colName := range []string{"Missing", "Dependencies", "Bucket", "Info"} {
				idx := indexRunes(headerRunes, colName)
				if idx > latestIdx && (missingIdx == -1 || idx < missingIdx) {
					missingIdx = idx
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

				if name != "" && !strings.HasPrefix(name, "-") && !strings.Contains(name, " ") && pm.ValidateScoopID(name) == nil {
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
		if len(matches) == 4 && pm.ValidateScoopID(matches[1]) == nil {
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
