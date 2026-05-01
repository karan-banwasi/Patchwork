package winget

import (
	"os/exec"
	"strings"
)

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
	cmd := exec.Command("winget", "upgrade")
	
	// Winget might return non-zero exit code if no updates are found or due to warnings
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// A very basic check to see if no updates were found
	if strings.Contains(outputStr, "No installed package found matching input criteria.") || 
	   strings.Contains(outputStr, "No applicable update found.") ||
	   strings.Contains(outputStr, "No packages found") {
		return []PackageUpdate{}, nil
	}

	return parseWingetOutput(outputStr), nil
}

// parseWingetOutput parses the tabular output from winget.
// Winget outputs variable-spaced columns, so we find the column offsets from the header.
func parseWingetOutput(output string) []PackageUpdate {
	var updates []PackageUpdate
	lines := strings.Split(output, "\n")
	
	headerLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Name") && strings.Contains(line, "Id") && strings.Contains(line, "Version") && strings.Contains(line, "Available") {
			headerLineIdx = i
			break
		}
	}

	if headerLineIdx == -1 || headerLineIdx+1 >= len(lines) {
		return updates
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
		return updates // Could not parse headers
	}

	// The row after headers should be a line of dashes, so we start at +2
	for i := headerLineIdx + 2; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n ")
		if line == "" {
			continue // skip empty lines or footer
		}

		// Some footers start with a number or are not package rows. But package rows will have enough length.
		if len(line) < availableIdx {
			continue // skip lines that are too short to be valid rows
		}

		// Clean the line from any preceding carriage returns that winget might leave
		if lastCR := strings.LastIndex(line, "\r"); lastCR != -1 {
			line = line[lastCR+1:]
		}

		// If the line is suddenly too short after cleaning
		if len(line) < availableIdx {
			continue
		}

		name := strings.TrimSpace(line[0:idIdx])
		
		id := ""
		if len(line) > versionIdx {
			id = strings.TrimSpace(line[idIdx:versionIdx])
		} else {
			id = strings.TrimSpace(line[idIdx:])
		}
		
		version := ""
		if len(line) > availableIdx {
			version = strings.TrimSpace(line[versionIdx:availableIdx])
		} else if len(line) > versionIdx {
			version = strings.TrimSpace(line[versionIdx:])
		}
		
		available := ""
		source := ""
		if len(line) > sourceIdx {
			available = strings.TrimSpace(line[availableIdx:sourceIdx])
			source = strings.TrimSpace(line[sourceIdx:])
		} else if len(line) > availableIdx {
			available = strings.TrimSpace(line[availableIdx:])
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

	return updates
}
