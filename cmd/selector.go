package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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

// IndexMultiSelector prompts the user with an interactive arrow-key/spacebar TUI checklist,
// tracking selections internally by candidate slice index.
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

// Select renders an interactive arrow-key and spacebar checkbox selector in the terminal
// and returns the chosen packages keyed by their candidate slice index.
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

	inFile, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(inFile.Fd())) {
		return nil, ErrNonInteractive
	}

	var outFile *os.File
	if f, ok := stdout.(*os.File); ok {
		outFile = f
	}

	restore, width, height, err := prepareInteractiveTerminal(inFile, outFile)
	if err != nil {
		return nil, ErrNonInteractive
	}
	defer restore()

	return runInteractiveSelectLoop(stdin, stdout, message, candidates, width, height)
}

func truncateRow(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string([]rune(s)[:maxRunes])
	}
	return string([]rune(s)[:maxRunes-3]) + "..."
}

// runInteractiveSelectLoop drives the arrow-key and spacebar multi-select UI over r and w.
// Selections are tracked strictly by 0-based index into candidates.
func runInteractiveSelectLoop(r io.Reader, w io.Writer, message string, candidates []pm.PackageUpdate, termWidth, termHeight int) ([]pm.PackageUpdate, error) {
	total := len(candidates)
	if total == 0 {
		return nil, nil
	}

	maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(candidates)
	rows := make([]string, total)
	for i, pkg := range candidates {
		rows[i] = formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
	}

	pageSize := 15
	if termHeight > 6 && termHeight-4 < pageSize {
		pageSize = termHeight - 4
	}
	if pageSize < 3 {
		pageSize = 3
	}

	maxRowWidth := 0
	if termWidth > 10 {
		maxRowWidth = termWidth - 8
	}

	checked := make([]bool, total)
	cursor := 0
	renderedLines := 0

	// Hide cursor while the interactive menu is active; always restore on return.
	fmt.Fprint(w, "\x1b[?25l")
	defer fmt.Fprint(w, "\x1b[?25h")

	clearMenu := func() {
		if renderedLines > 0 {
			fmt.Fprintf(w, "\x1b[%dA\r\x1b[J", renderedLines)
			renderedLines = 0
		}
	}

	renderFrame := func() {
		if renderedLines > 0 {
			fmt.Fprintf(w, "\x1b[%dA\r", renderedLines)
		}

		var b strings.Builder
		lines := 0

		// Header prompt line
		b.WriteString("\x1b[2K\x1b[1;32m?\x1b[0m \x1b[1m")
		b.WriteString(message)
		b.WriteString("\x1b[0m  \x1b[36m[Use arrows to move, space to select, <right> to all, <left> to none, enter to confirm]\x1b[0m\r\n")
		lines++

		// Calculate visible slice window [startIdx, endIdx)
		startIdx := 0
		endIdx := total
		if total > pageSize {
			startIdx = cursor - pageSize/2
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx = startIdx + pageSize
			if endIdx > total {
				endIdx = total
				startIdx = endIdx - pageSize
			}
		}

		for i := startIdx; i < endIdx; i++ {
			b.WriteString("\x1b[2K")
			if i == cursor {
				b.WriteString("\x1b[1;36m>\x1b[0m")
			} else {
				b.WriteString(" ")
			}
			if checked[i] {
				b.WriteString("\x1b[32m [✓] \x1b[0m")
			} else {
				b.WriteString(" [ ] ")
			}
			b.WriteString(" ")
			b.WriteString(truncateRow(rows[i], maxRowWidth))
			b.WriteString("\r\n")
			lines++
		}

		if total > pageSize {
			b.WriteString(fmt.Sprintf("\x1b[2K  \x1b[90m(Move up/down to reveal more items — %d/%d)\x1b[0m\r\n", cursor+1, total))
			lines++
		}

		fmt.Fprint(w, b.String())
		renderedLines = lines
	}

	renderConfirmed := func() {
		clearMenu()
		var b strings.Builder
		b.WriteString("\x1b[1;32m?\x1b[0m \x1b[1m")
		b.WriteString(message)
		b.WriteString("\x1b[0m\r\n")
		for i := 0; i < total; i++ {
			if checked[i] {
				b.WriteString("  \x1b[36m")
				b.WriteString(rows[i])
				b.WriteString("\x1b[0m\r\n")
			}
		}
		fmt.Fprint(w, b.String())
	}

	renderFrame()

	var buf [32]byte
	for {
		n, err := r.Read(buf[:])
		if n == 0 {
			if err != nil {
				clearMenu()
				if errors.Is(err, io.EOF) {
					return nil, ErrSelectionCanceled
				}
				return nil, err
			}
			continue
		}

		for i := 0; i < n; {
			ch := buf[i]

			// ANSI escape sequences (\x1b[A, \x1bOA, etc.) or standalone Esc
			if ch == 0x1b {
				if i+2 < n && (buf[i+1] == '[' || buf[i+1] == 'O') {
					code := buf[i+2]
					i += 3
					switch code {
					case 'A': // Up arrow
						cursor = (cursor - 1 + total) % total
					case 'B': // Down arrow
						cursor = (cursor + 1) % total
					case 'C': // Right arrow -> select all
						for idx := range checked {
							checked[idx] = true
						}
					case 'D': // Left arrow -> select none
						for idx := range checked {
							checked[idx] = false
						}
					case 'H': // Home
						cursor = 0
					case 'F': // End
						cursor = total - 1
					default:
						// Consume trailing tilde for sequences like \x1b[5~ (PageUp) / \x1b[6~ (PageDown)
						for i < n && buf[i-1] != '~' && (buf[i] >= '0' && buf[i] <= '9' || buf[i] == ';' || buf[i] == '~') {
							if buf[i] == '~' {
								switch code {
								case '5': // PageUp
									cursor -= pageSize
									if cursor < 0 {
										cursor = 0
									}
								case '6': // PageDown
									cursor += pageSize
									if cursor >= total {
										cursor = total - 1
									}
								}
								i++
								break
							}
							i++
						}
					}
					renderFrame()
					continue
				}
				// Standalone Esc cancels selection
				clearMenu()
				return nil, ErrSelectionCanceled
			}

			// Windows legacy scan-code prefix (0xE0 or 0x00)
			if (ch == 0xE0 || ch == 0x00) && i+1 < n {
				scan := buf[i+1]
				i += 2
				switch scan {
				case 0x48: // Up
					cursor = (cursor - 1 + total) % total
				case 0x50: // Down
					cursor = (cursor + 1) % total
				case 0x4D: // Right -> select all
					for idx := range checked {
						checked[idx] = true
					}
				case 0x4B: // Left -> select none
					for idx := range checked {
						checked[idx] = false
					}
				}
				renderFrame()
				continue
			}

			i++
			switch ch {
			case 0x03, 0x04, 'q', 'Q': // Ctrl+C, Ctrl+D, q
				clearMenu()
				return nil, ErrSelectionCanceled
			case ' ': // Space toggles current item
				checked[cursor] = !checked[cursor]
				renderFrame()
			case 'k', 'K': // Vim up
				cursor = (cursor - 1 + total) % total
				renderFrame()
			case 'j', 'J': // Vim down
				cursor = (cursor + 1) % total
				renderFrame()
			case 'a', 'A': // Select all
				for idx := range checked {
					checked[idx] = true
				}
				renderFrame()
			case 'n', 'N': // Select none
				for idx := range checked {
					checked[idx] = false
				}
				renderFrame()
			case '\r', '\n': // Enter confirms selection
				// Swallow immediate companion in CRLF if present in same buffer
				if ch == '\r' && i < n && buf[i] == '\n' {
					i++
				}
				renderConfirmed()
				var selected []pm.PackageUpdate
				for idx, isChecked := range checked {
					if isChecked {
						selected = append(selected, candidates[idx])
					}
				}
				return selected, nil
			}
		}
	}
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
