package winget

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

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

// PackageUpdate represents an available update for a package.
type PackageUpdate struct {
	Name             string
	Id               string
	Version          string
	AvailableVersion string
	Source           string
}

// GetAvailableUpdates runs `winget upgrade` to fetch packages that have updates available.
func GetAvailableUpdates() ([]PackageUpdate, error) {
	exe, err := wingetPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "upgrade")
	
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// We deliberately ignore stderr to prevent warnings from corrupting the table output
	
	// Winget might return non-zero exit code if no updates are found or due to warnings
	err = cmd.Run()
	outputStr := stdout.String()

	// A very basic check to see if no updates were found
	if strings.Contains(outputStr, "No installed package found matching input criteria.") || 
	   strings.Contains(outputStr, "No applicable update found.") ||
	   strings.Contains(outputStr, "No packages found") {
		return []PackageUpdate{}, nil
	}

	updates, parseErr := parseWingetOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	// If winget failed and we couldn't parse any updates, propagate the error.
	if err != nil && len(updates) == 0 {
		return nil, fmt.Errorf("winget execution failed: %w (output: %s)", err, strings.TrimSpace(outputStr))
	}

	return updates, nil
}

// parseWingetOutput parses the tabular output from winget.
// Winget outputs variable-spaced columns, so we find the column offsets from the header.
func parseWingetOutput(output string) ([]PackageUpdate, error) {
	var updates []PackageUpdate
	
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
	
	// Winget sometimes outputs its progress spinner on the same line as the headers!
	// We need to slice off anything before the actual 'Name' column starts.
	nameIdx := strings.Index(headerLine, "Name")
	if nameIdx != -1 {
		headerLine = headerLine[nameIdx:]
	}

	idIdx := strings.Index(headerLine, "Id")
	versionIdx := strings.Index(headerLine, "Version")
	availableIdx := strings.Index(headerLine, "Available")
	sourceIdx := strings.Index(headerLine, "Source")

	if idIdx == -1 || versionIdx == -1 || availableIdx == -1 || sourceIdx == -1 {
		return updates, nil // Could not parse headers
	}
	
	// Validate that columns are in the expected order
	if !(idIdx > 0 && versionIdx > idIdx && availableIdx > versionIdx && sourceIdx > availableIdx) {
		return nil, fmt.Errorf("winget header columns are in an unexpected order")
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
			updates = append(updates, PackageUpdate{
				Name:             name,
				Id:               id,
				Version:          version,
				AvailableVersion: available,
				Source:           source,
			})
		}
	}

	return updates, nil
}
