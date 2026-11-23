package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestInteractiveSelectLoop_ArrowsAndSpace(t *testing.T) {
	candidates := []pm.PackageUpdate{
		{Id: "pkg0", Name: "Alpha", Version: "1.0.0", AvailableVersion: "1.1.0", ManagerName: "winget"},
		{Id: "pkg1", Name: "Bravo", Version: "2.0.0", AvailableVersion: "2.1.0", ManagerName: "scoop"},
		{Id: "pkg2", Name: "Charlie", Version: "3.0.0", AvailableVersion: "3.1.0", ManagerName: "npm"},
	}

	// Down arrow -> Space (select idx 1) -> Down arrow -> Space (select idx 2) -> Up arrow -> Space (unselect idx 1) -> Up arrow -> Space (select idx 0) -> Enter
	input := "\x1b[B \x1b[B \x1b[A \x1b[A \r"
	var out bytes.Buffer

	selected, err := runInteractiveSelectLoop(strings.NewReader(input), &out, "Select packages to upgrade:", candidates, 120, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected packages, got %d: %+v", len(selected), selected)
	}
	if selected[0].Id != "pkg0" || selected[1].Id != "pkg2" {
		t.Errorf("expected pkg0 and pkg2, got %+v", selected)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "[✓]") {
		t.Errorf("expected rendered output to contain checked indicator [✓]")
	}
}

func TestInteractiveSelectLoop_RightAllAndLeftNone(t *testing.T) {
	candidates := []pm.PackageUpdate{
		{Id: "pkg0", Name: "Alpha", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
		{Id: "pkg1", Name: "Bravo", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
	}

	// Right arrow (select all) -> Enter
	var out bytes.Buffer
	selected, err := runInteractiveSelectLoop(strings.NewReader("\x1b[C\r"), &out, "Select:", candidates, 120, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected all 2 packages selected, got %d", len(selected))
	}

	// Right arrow (select all) -> Left arrow (select none) -> Space (select idx 0) -> Enter
	out.Reset()
	selected, err = runInteractiveSelectLoop(strings.NewReader("\x1b[C\x1b[D \r"), &out, "Select:", candidates, 120, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 1 || selected[0].Id != "pkg0" {
		t.Fatalf("expected only pkg0 selected, got %+v", selected)
	}
}

func TestInteractiveSelectLoop_DuplicateDisplayLabelsKeyedByIndex(t *testing.T) {
	// Two distinct candidates that produce identical formatted display rows except internal metadata
	candidates := []pm.PackageUpdate{
		{Id: "dup.id", Name: "Duplicate", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
		{Id: "dup.id", Name: "Duplicate", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
	}

	// Select both items (Space, Down, Space, Enter) -> both must be returned by index without map collision
	var out bytes.Buffer
	selected, err := runInteractiveSelectLoop(strings.NewReader(" \x1b[B \r"), &out, "Select:", candidates, 120, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 distinct index selections even with identical labels, got %d", len(selected))
	}
}

func TestInteractiveSelectLoop_CancelKeys(t *testing.T) {
	candidates := []pm.PackageUpdate{
		{Id: "pkg0", Name: "Alpha", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
	}

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"Ctrl+C", "\x03"},
		{"Standalone Esc", "\x1b"},
		{"Quit q", "q"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			_, err := runInteractiveSelectLoop(strings.NewReader(tc.input), &out, "Select:", candidates, 120, 24)
			if !errors.Is(err, ErrSelectionCanceled) {
				t.Errorf("expected ErrSelectionCanceled for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestMockMultiSelector(t *testing.T) {
	candidates := []pm.PackageUpdate{
		{Id: "pkg0", Name: "Pkg0"},
		{Id: "pkg1", Name: "Pkg1"},
		{Id: "pkg2", Name: "Pkg2"},
	}

	mock := &MockMultiSelector{
		SelectedIndices: []int{0, 2},
	}

	selected, err := mock.Select("test prompt", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(selected))
	}
	if selected[0].Id != "pkg0" || selected[1].Id != "pkg2" {
		t.Errorf("unexpected selected packages: %+v", selected)
	}

	mockErr := &MockMultiSelector{
		Err: errors.New("simulated abort"),
	}
	_, err = mockErr.Select("prompt", candidates)
	if err == nil || err.Error() != "simulated abort" {
		t.Errorf("expected simulated abort error, got: %v", err)
	}
}
