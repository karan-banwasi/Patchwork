package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"golang.org/x/term"
)

var (
	// ErrSelectionCanceled is returned when the user cancels package selection.
	ErrSelectionCanceled = errors.New("selection canceled by user")
	// ErrNonInteractive is returned when interactive selection is attempted in a non-TTY environment.
	ErrNonInteractive = errors.New("interactive selection requires an interactive terminal (TTY); pass --all or specify package IDs")
)

// MultiSelector abstracts interactive package update checklist selection.
type MultiSelector interface {
	Select(message string, candidates []pm.PackageUpdate) ([]pm.PackageUpdate, error)
}

// IndexMultiSelector prompts the user for package selection by index numbers.
type IndexMultiSelector struct {
	Stdin  io.Reader
	Stdout io.Writer
}

func NewIndexMultiSelector(stdin io.Reader, stdout io.Writer) *IndexMultiSelector {
	return &IndexMultiSelector{
		Stdin:  stdin,
		Stdout: stdout,
	}
}

// isInteractive reports whether r is connected to a terminal.
func isInteractive(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// Select prompts the user for packages using index numbers and returns the selected items.
func (s *IndexMultiSelector) Select(message string, candidates []pm.PackageUpdate) ([]pm.PackageUpdate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	stdin := s.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := s.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	if !isInteractive(stdin) {
		return nil, ErrNonInteractive
	}

	maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(candidates)

	fmt.Fprintln(stdout, message)
	for i, pkg := range candidates {
		row := formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
		fmt.Fprintf(stdout, "  [%d] %s\n", i+1, row)
	}
	fmt.Fprintf(stdout, "Enter package numbers to upgrade (e.g. 1, 3-5, 'all', or press Enter to cancel): ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, ErrSelectionCanceled
	}

	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return nil, nil
	}

	if strings.EqualFold(line, "all") {
		selected := make([]pm.PackageUpdate, len(candidates))
		copy(selected, candidates)
		return selected, nil
	}

	indices, err := parseSelectionIndices(line, len(candidates))
	if err != nil {
		return nil, err
	}

	var selected []pm.PackageUpdate
	for _, idx := range indices {
		selected = append(selected, candidates[idx])
	}
	return selected, nil
}

// parseSelectionIndices parses input like "1, 3-5, 7" into sorted unique 0-based slice indices.
func parseSelectionIndices(input string, total int) ([]int, error) {
	seen := make(map[int]bool)
	parts := strings.Split(input, ",")
	for _, p := range parts {
		token := strings.TrimSpace(p)
		if token == "" {
			continue
		}
		if strings.Contains(token, "-") {
			rangeParts := strings.Split(token, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range specification %q", token)
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err1 != nil || err2 != nil || start > end || start < 1 || end > total {
				return nil, fmt.Errorf("invalid range %q (must be between 1 and %d)", token, total)
			}
			for i := start; i <= end; i++ {
				seen[i-1] = true
			}
		} else {
			num, err := strconv.Atoi(token)
			if err != nil || num < 1 || num > total {
				return nil, fmt.Errorf("invalid package number %q (must be between 1 and %d)", token, total)
			}
			seen[num-1] = true
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	result := make([]int, 0, len(seen))
	for idx := range seen {
		result = append(result, idx)
	}
	sort.Ints(result)
	return result, nil
}

// MockMultiSelector is a mock MultiSelector for headless automated tests.
type MockMultiSelector struct {
	SelectedIndices []int
	Err             error
}

func (m *MockMultiSelector) Select(message string, candidates []pm.PackageUpdate) ([]pm.PackageUpdate, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	var out []pm.PackageUpdate
	for _, idx := range m.SelectedIndices {
		if idx >= 0 && idx < len(candidates) {
			out = append(out, candidates[idx])
		}
	}
	return out, nil
}

var defaultMultiSelector MultiSelector = NewIndexMultiSelector(nil, nil)

// SetDefaultSelectorForTest overrides the default selector for unit testing.
func SetDefaultSelectorForTest(sel MultiSelector) {
	if sel == nil {
		defaultMultiSelector = NewIndexMultiSelector(nil, nil)
	} else {
		defaultMultiSelector = sel
	}
}
