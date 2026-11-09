package cmd

import (
	"errors"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestParseSelectionIndices(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		total     int
		expected  []int
		expectErr bool
	}{
		{"single index", "1", 3, []int{0}, false},
		{"comma separated", "1, 3", 3, []int{0, 2}, false},
		{"range", "2-4", 5, []int{1, 2, 3}, false},
		{"mixed comma and range", "1, 3-4", 5, []int{0, 2, 3}, false},
		{"empty string", "", 5, nil, false},
		{"out of bounds high", "6", 5, nil, true},
		{"out of bounds zero", "0", 5, nil, true},
		{"invalid range inverted", "4-2", 5, nil, true},
		{"invalid range format", "1-2-3", 5, nil, true},
		{"non-numeric", "abc", 5, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectionIndices(tt.input, tt.total)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("length mismatch: got %v, want %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("at index %d: got %d, want %d", i, got[i], tt.expected[i])
				}
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
