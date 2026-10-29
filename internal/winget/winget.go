package winget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

// WingetNoApplicableUpgradeExitCode is APPINSTALLER_CLI_ERROR_NO_APPLICABLE_UPGRADE (0x8A15002B / -1978335189).
const WingetNoApplicableUpgradeExitCode = 0x8A15002B

// WingetManager implements the pm.Manager interface for Windows Package Manager (winget).
type WingetManager struct {
	AcceptAgreements bool
	runner           pm.Runner
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
}

// NewWingetManager creates a new instance of WingetManager.
func NewWingetManager() *WingetManager {
	return &WingetManager{
		AcceptAgreements: true,
		runner:           pm.ExecRunner{},
		stdout:           io.Discard,
		stderr:           io.Discard,
	}
}

func (w *WingetManager) Name() string {
	return "winget"
}

func (w *WingetManager) Aliases() []string {
	return nil
}

func (w *WingetManager) SetIO(stdin io.Reader, stdout, stderr io.Writer) {
	w.stdin = stdin
	if stdout != nil {
		w.stdout = stdout
	} else {
		w.stdout = io.Discard
	}
	if stderr != nil {
		w.stderr = stderr
	} else {
		w.stderr = io.Discard
	}
}

func (w *WingetManager) ValidateID(id string) error {
	return pm.ValidateWingetID(id)
}

// ShouldDefer reports whether upgrading id may close or restart the host shell or terminal.
func (w *WingetManager) ShouldDefer(id string) bool {
	return shouldOmitSilent(id)
}

// Path resolves the absolute path to the winget executable.
func (w *WingetManager) Path() (string, error) {
	return wingetPath()
}

// Version returns the installed winget version string.
func (w *WingetManager) Version(ctx context.Context) (string, error) {
	exe, err := wingetPath()
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (w *WingetManager) IsAvailable(ctx context.Context) bool {
	_, err := wingetPath()
	return err == nil
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

func isNoApplicableUpgradeExitCode(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	code := uint32(exitErr.ExitCode())
	return code == WingetNoApplicableUpgradeExitCode
}

// GetAvailableUpdates runs `winget upgrade` to fetch packages that have updates available.
func (w *WingetManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	exe, err := wingetPath()
	if err != nil {
		return nil, err
	}

	args := []string{"upgrade", "--accept-source-agreements"}
	var stdout, stderr bytes.Buffer
	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	err = runner.Run(ctx, exe, args, nil, &stdout, &stderr)
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	if isNoApplicableUpgradeExitCode(err) {
		return []pm.PackageUpdate{}, nil
	}

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

// GetAvailableUpdates is a package-level helper for convenience.
func GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return NewWingetManager().GetAvailableUpdates(ctx)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHFABCDJsuhl?]`)

// runeDisplayWidth returns the terminal column width of a rune (2 for full-width CJK runes, 1 otherwise).
func runeDisplayWidth(r rune) int {
	if (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) {
		return 2
	}
	return 1
}

// indexDisplayWidth returns the starting display column offset of sub within runes, or -1 if not found.
func indexDisplayWidth(runes []rune, sub string) int {
	subRunes := []rune(sub)
	if len(subRunes) == 0 {
		return 0
	}
	col := 0
	for i := 0; i <= len(runes)-len(subRunes); i++ {
		match := true
		for j := 0; j < len(subRunes); j++ {
			if runes[i+j] != subRunes[j] {
				match = false
				break
			}
		}
		if match {
			return col
		}
		col += runeDisplayWidth(runes[i])
	}
	return -1
}

// findWordStartDisplayWidths returns the starting display column offsets of words in runes.
func findWordStartDisplayWidths(runes []rune) []int {
	var indices []int
	inWord := false
	col := 0
	for _, r := range runes {
		if r != ' ' && r != '\t' && r != '\r' {
			if !inWord {
				inWord = true
				indices = append(indices, col)
			}
		} else {
			inWord = false
		}
		col += runeDisplayWidth(r)
	}
	return indices
}

// sliceByDisplayWidth extracts the substring of runes spanning display columns [startCol, endCol).
// If endCol < 0, slices from startCol to the end of runes.
func sliceByDisplayWidth(runes []rune, startCol, endCol int) string {
	col := 0
	startIdx := len(runes)
	endIdx := len(runes)
	started := false
	for i, r := range runes {
		if !started && col >= startCol {
			startIdx = i
			started = true
		}
		if endCol >= 0 && col >= endCol {
			endIdx = i
			break
		}
		col += runeDisplayWidth(r)
	}
	if !started || startIdx >= endIdx {
		return ""
	}
	return string(runes[startIdx:endIdx])
}

// parseWingetOutput parses the tabular output from winget.
// Winget outputs variable-spaced columns, so we find the column offsets from the header.
func parseWingetOutput(output string) ([]pm.PackageUpdate, error) {
	var updates []pm.PackageUpdate

	cleanOutput := ansiEscape.ReplaceAllString(output, "")
	lines := strings.Split(cleanOutput, "\n")

	headerLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Name") && (strings.Contains(line, "Id") || strings.Contains(line, "ID")) && strings.Contains(line, "Version") && strings.Contains(line, "Available") {
			headerLineIdx = i
			break
		}
	}

	// Fallback for localized Winget output: look for the table separator line of dashes
	if headerLineIdx == -1 {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "---") && strings.Count(trimmed, "-") > 10 {
				if i > 0 {
					headerLineIdx = i - 1
					break
				}
			}
		}
	}

	if headerLineIdx == -1 || headerLineIdx+1 >= len(lines) {
		return updates, nil
	}

	headerLine := lines[headerLineIdx]
	if lastCR := strings.LastIndex(headerLine, "\r"); lastCR != -1 {
		headerLine = headerLine[lastCR+1:]
	}
	headerRunes := []rune(headerLine)

	nameIdx := indexDisplayWidth(headerRunes, "Name")
	idIdx := indexDisplayWidth(headerRunes, "Id")
	if idIdx == -1 {
		idIdx = indexDisplayWidth(headerRunes, "ID")
	}
	versionIdx := indexDisplayWidth(headerRunes, "Version")
	availableIdx := indexDisplayWidth(headerRunes, "Available")
	sourceIdx := indexDisplayWidth(headerRunes, "Source")

	if nameIdx == -1 || idIdx == -1 || versionIdx == -1 || availableIdx == -1 || sourceIdx == -1 {
		// Use word boundary offsets for localized headers
		wordIndices := findWordStartDisplayWidths(headerRunes)
		if len(wordIndices) < 5 {
			return updates, nil // Could not parse localized headers
		}
		// If a progress spinner character (e.g. "/", "-", "\") precedes the 5 column headers, use the last 5 words
		offset := 0
		if len(wordIndices) == 6 && wordIndices[1]-wordIndices[0] <= 3 {
			offset = 1
		}
		nameIdx = wordIndices[offset]
		idIdx = wordIndices[offset+1]
		versionIdx = wordIndices[offset+2]
		availableIdx = wordIndices[offset+3]
		sourceIdx = wordIndices[offset+4]
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

		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			continue
		}

		runes := []rune(line)
		totalWidth := 0
		for _, r := range runes {
			totalWidth += runeDisplayWidth(r)
		}

		// Some footers start with a number or are not package rows. But package rows will have enough length.
		if totalWidth < availableIdx {
			continue // skip lines that are too short to be valid rows
		}

		name := strings.TrimSpace(sliceByDisplayWidth(runes, 0, idIdx))
		id := strings.TrimSpace(sliceByDisplayWidth(runes, idIdx, versionIdx))
		version := strings.TrimSpace(sliceByDisplayWidth(runes, versionIdx, availableIdx))
		available := ""
		source := ""
		if totalWidth > sourceIdx {
			available = strings.TrimSpace(sliceByDisplayWidth(runes, availableIdx, sourceIdx))
			source = strings.TrimSpace(sliceByDisplayWidth(runes, sourceIdx, -1))
		} else {
			available = strings.TrimSpace(sliceByDisplayWidth(runes, availableIdx, -1))
		}

		// Ignore non-package lines that might get caught at the bottom
		// Winget package IDs do not contain spaces and must be valid winget IDs
		isRepeatHeader := (strings.EqualFold(name, "Name") && (strings.EqualFold(id, "Id") || strings.EqualFold(id, "ID"))) ||
			strings.EqualFold(id, "ID")
		if name != "" && id != "" && !strings.HasPrefix(name, "-") && !strings.Contains(id, " ") &&
			!strings.EqualFold(version, "Unknown") && !isRepeatHeader && pm.ValidateWingetID(id) == nil {
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
