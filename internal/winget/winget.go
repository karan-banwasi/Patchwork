package winget

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

// WingetManager implements the pm.Manager interface for Windows Package Manager (winget).
type WingetManager struct{}

// NewWingetManager creates a new instance of WingetManager.
func NewWingetManager() *WingetManager {
	return &WingetManager{}
}

func (w *WingetManager) Name() string {
	return "winget"
}

func (w *WingetManager) IsAvailable(ctx context.Context) bool {
	_, err := wingetPath()
	return err == nil
}

func (w *WingetManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return GetAvailableUpdates(ctx)
}

func (w *WingetManager) UpgradePackage(ctx context.Context, id string) error {
	return UpgradePackage(ctx, id)
}

func (w *WingetManager) UpgradeAll(ctx context.Context) error {
	return UpgradeAll(ctx)
}

// wingetPath resolves the absolute path to the winget executable
// in a trusted location to prevent PATH hijacking.
func wingetPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return exec.LookPath("winget.exe")
	}
	path := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
	if _, err := os.Stat(path); err != nil {
		if fallbackPath, errLook := exec.LookPath("winget.exe"); errLook == nil {
			return fallbackPath, nil
		}
		return "", fmt.Errorf("winget.exe not found in trusted location or PATH: %w", err)
	}
	return path, nil
}

// GetAvailableUpdates runs `winget upgrade` to fetch packages that have updates available.
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := wingetPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, "upgrade")
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Winget might return non-zero exit code if no updates are found or due to warnings
	err = cmd.Run()
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	// A very basic check to see if no updates were found
	if strings.Contains(outputStr, "No installed package found matching input criteria.") || 
	   strings.Contains(outputStr, "No applicable update found.") ||
	   strings.Contains(outputStr, "No packages found") {
		return []pm.PackageUpdate{}, nil
	}

	updates, parseErr := parseWingetOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	// If winget failed and we couldn't parse any updates, propagate the error.
	if err != nil && len(updates) == 0 {
		errMsg := fmt.Sprintf("winget execution failed: %v", err)
		if stderrStr != "" {
			errMsg += fmt.Sprintf(" (stderr: %s)", stderrStr)
		} else if strings.TrimSpace(outputStr) != "" {
			errMsg += fmt.Sprintf(" (stdout: %s)", strings.TrimSpace(outputStr))
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return updates, nil
}

// parseWingetOutput parses the tabular output from winget.
// Winget outputs variable-spaced columns, so we find the column offsets from the header.
func parseWingetOutput(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate
	
	var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHFABCDJsuhl?]`)
	cleanOutput := ansiEscape.ReplaceAllString(output, "")
	
	lines := strings.Split(cleanOutput, "\n")
	
	headerLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Name") && strings.Contains(line, "Id") && strings.Contains(line, "Version") && strings.Contains(line, "Available") {
			headerLineIdx = i
			break
		}
	}

	if headerLineIdx == -1 || headerLineIdx+1 >= len(lines) {
		return updates, nil
	}

	headerLine := lines[headerLineIdx]
	
	nameIdx := strings.Index(headerLine, "Name")
	idIdx := strings.Index(headerLine, "Id")
	versionIdx := strings.Index(headerLine, "Version")
	availableIdx := strings.Index(headerLine, "Available")
	sourceIdx := strings.Index(headerLine, "Source")

	if nameIdx == -1 || idIdx == -1 || versionIdx == -1 || availableIdx == -1 || sourceIdx == -1 {
		return updates, nil // Could not parse headers
	}
	
	// Validate that columns are in the expected order
	if !(nameIdx >= 0 && idIdx > nameIdx && versionIdx > idIdx && availableIdx > versionIdx && sourceIdx > availableIdx) {
		return nil, fmt.Errorf("winget header columns are in an unexpected order")
	}

	// Winget sometimes outputs its progress spinner or characters on the same line as the headers.
	// Normalize column indices relative to column 0 where "Name" starts in data rows.
	if nameIdx > 0 {
		idIdx -= nameIdx
		versionIdx -= nameIdx
		availableIdx -= nameIdx
		sourceIdx -= nameIdx
	}

	// The row after headers should be a line of dashes, so we start at +2
	for i := headerLineIdx + 2; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n ")
		if line == "" {
			continue // skip empty lines or footer
		}

		// Clean the line from any preceding carriage returns that winget might leave
		if lastCR := strings.LastIndex(line, "\r"); lastCR != -1 {
			line = line[lastCR+1:]
		}

		runes := []rune(line)

		// Some footers start with a number or are not package rows. But package rows will have enough length.
		if len(runes) < availableIdx {
			continue // skip lines that are too short to be valid rows
		}

		name := strings.TrimSpace(string(runes[0:idIdx]))
		
		id := ""
		if len(runes) > versionIdx {
			id = strings.TrimSpace(string(runes[idIdx:versionIdx]))
		} else {
			id = strings.TrimSpace(string(runes[idIdx:]))
		}
		
		version := ""
		if len(runes) > availableIdx {
			version = strings.TrimSpace(string(runes[versionIdx:availableIdx]))
		} else if len(runes) > versionIdx {
			version = strings.TrimSpace(string(runes[versionIdx:]))
		}
		
		available := ""
		source := ""
		if len(runes) > sourceIdx {
			available = strings.TrimSpace(string(runes[availableIdx:sourceIdx]))
			source = strings.TrimSpace(string(runes[sourceIdx:]))
		} else if len(runes) > availableIdx {
			available = strings.TrimSpace(string(runes[availableIdx:]))
		}

		// Ignore non-package lines that might get caught at the bottom
		// Winget package IDs do not contain spaces
		if name != "" && id != "" && !strings.HasPrefix(name, "-") && !strings.Contains(id, " ") {
			updates = append(updates, pm.PackageUpdate{
				Name:             name,
				Id:               id,
				Version:          version,
				AvailableVersion: available,
				Source:           source,
				ManagerName:      "winget",
			})
		}
	}

	return updates, nil
}
