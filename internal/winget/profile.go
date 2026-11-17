package winget

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Lister    = (*WingetManager)(nil)
	_ pm.Installer = (*WingetManager)(nil)
)

func isMockRunner(r pm.Runner) bool {
	if r == nil {
		return false
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return !isExec && !isExecPtr
}

func (w *WingetManager) resolveExe() (string, error) {
	exe, err := wingetPath()
	if err != nil {
		if isMockRunner(w.runner) {
			return "winget", nil
		}
		return "", err
	}
	return exe, nil
}

// ListInstalled runs `winget list --accept-source-agreements` and parses installed packages.
func (w *WingetManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	exe, err := w.resolveExe()
	if err != nil {
		return nil, err
	}

	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stdout, stderr bytes.Buffer
	err = runner.Run(ctx, exe, []string{"list", "--accept-source-agreements"}, nil, &stdout, &stderr)
	outputStr := stdout.String()
	stderrStr := strings.TrimSpace(stderr.String())

	if strings.Contains(outputStr, "No installed package found matching input criteria.") ||
		strings.Contains(outputStr, "No packages found") {
		return []pm.InstalledPackage{}, nil
	}

	pkgs, parseErr := parseWingetListOutput(outputStr)
	if parseErr != nil {
		return nil, parseErr
	}

	if err != nil && len(pkgs) == 0 {
		errMsg := fmt.Sprintf("winget list failed: %v", err)
		if stderrStr != "" {
			errMsg += fmt.Sprintf(" (stderr: %s)", stderrStr)
		} else if strings.TrimSpace(outputStr) != "" {
			errMsg += fmt.Sprintf(" (stdout: %s)", strings.TrimSpace(outputStr))
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return pkgs, nil
}

func parseWingetListOutput(output string) ([]pm.InstalledPackage, error) {
	var pkgs []pm.InstalledPackage

	cleanOutput := ansiEscape.ReplaceAllString(output, "")
	cleanOutput = strings.ReplaceAll(cleanOutput, "\r\n", "\n")
	lines := strings.Split(cleanOutput, "\n")

	headerLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Name") && (strings.Contains(line, "Id") || strings.Contains(line, "ID")) && strings.Contains(line, "Version") {
			headerLineIdx = i
			break
		}
	}

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
		return []pm.InstalledPackage{}, nil
	}

	headerLine := strings.TrimRight(lines[headerLineIdx], "\r\n ")
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

	if nameIdx == -1 || idIdx == -1 || versionIdx == -1 {
		wordIndices := findWordStartDisplayWidths(headerRunes)
		if len(wordIndices) < 3 {
			return []pm.InstalledPackage{}, nil
		}
		offset := 0
		if len(wordIndices) >= 4 && wordIndices[1]-wordIndices[0] <= 3 {
			offset = 1
		}
		if len(wordIndices)-offset < 3 {
			return []pm.InstalledPackage{}, nil
		}
		nameIdx = wordIndices[offset]
		idIdx = wordIndices[offset+1]
		versionIdx = wordIndices[offset+2]
		if len(wordIndices)-offset >= 5 {
			availableIdx = wordIndices[offset+3]
			sourceIdx = wordIndices[offset+4]
		} else if len(wordIndices)-offset == 4 {
			sourceIdx = wordIndices[offset+3]
		}
	}

	if !(nameIdx >= 0 && idIdx > nameIdx && versionIdx > idIdx) {
		return nil, fmt.Errorf("winget list header columns are in an unexpected order")
	}

	if nameIdx > 0 {
		idIdx -= nameIdx
		versionIdx -= nameIdx
		if availableIdx > 0 {
			availableIdx -= nameIdx
		}
		if sourceIdx > 0 {
			sourceIdx -= nameIdx
		}
	}

	// Determine end column for Version: Available if present, else Source if present, else end of line.
	verEndIdx := -1
	if availableIdx > versionIdx {
		verEndIdx = availableIdx
	} else if sourceIdx > versionIdx {
		verEndIdx = sourceIdx
	}

	for i := headerLineIdx + 2; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n ")
		if line == "" {
			continue
		}
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
		if totalWidth < versionIdx {
			continue
		}

		name := strings.TrimSpace(sliceByDisplayWidth(runes, 0, idIdx))
		id := strings.TrimSpace(sliceByDisplayWidth(runes, idIdx, versionIdx))
		ver := ""
		if verEndIdx > 0 && totalWidth > verEndIdx {
			ver = strings.TrimSpace(sliceByDisplayWidth(runes, versionIdx, verEndIdx))
		} else {
			ver = strings.TrimSpace(sliceByDisplayWidth(runes, versionIdx, -1))
		}

		isRepeatHeader := (strings.EqualFold(name, "Name") && (strings.EqualFold(id, "Id") || strings.EqualFold(id, "ID"))) ||
			strings.EqualFold(id, "ID")
		if name == "" || id == "" || strings.HasPrefix(name, "-") || strings.Contains(id, " ") || isRepeatHeader {
			continue
		}
		if pm.ValidateWingetID(id) != nil {
			continue
		}
		if strings.EqualFold(ver, "Unknown") {
			ver = ""
		}

		pkgs = append(pkgs, pm.InstalledPackage{
			Name:        name,
			ID:          id,
			Version:     ver,
			ManagerName: "winget",
		})
	}

	sort.Slice(pkgs, func(i, j int) bool {
		return strings.ToLower(pkgs[i].ID) < strings.ToLower(pkgs[j].ID)
	})
	return pkgs, nil
}

// InstallPackage installs a specific winget package ID and optional version.
func (w *WingetManager) InstallPackage(ctx context.Context, id, version string) error {
	if err := w.ValidateID(id); err != nil {
		return err
	}
	if err := pm.ValidateOptionalVersion(version); err != nil {
		return err
	}

	exe, err := w.resolveExe()
	if err != nil {
		return err
	}

	args := []string{"install", "--id", id, "--exact"}
	if version != "" {
		args = append(args, "--version", version)
	}
	if w.AcceptAgreements {
		if !shouldOmitSilent(id) {
			args = append(args, "--silent")
		}
		args = append(args, "--accept-source-agreements", "--accept-package-agreements")
	}

	stdout := w.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := w.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	runner := w.runner
	if runner == nil {
		runner = pm.ExecRunner{}
	}

	var stderrBuf bytes.Buffer
	if err := runner.Run(ctx, exe, args, w.stdin, stdout, io.MultiWriter(stderr, &stderrBuf)); err != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return fmt.Errorf("winget install failed for %s: %w (%s)", id, err, stderrStr)
		}
		return fmt.Errorf("winget install failed for %s: %w", id, err)
	}
	return nil
}
