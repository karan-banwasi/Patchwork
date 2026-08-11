package cmd

import (
	"testing"
	"github.com/karan-banwasi/patchwork/internal/winget"
)

func TestGetPackageColumnWidths(t *testing.T) {
	updates := []winget.PackageUpdate{
		{
			Name:             "Git",
			Id:               "Git.Git",
			Version:          "2.55.0.2",
			AvailableVersion: "2.55.0.3",
		},
		{
			Name:             "Visual Studio Code",
			Id:               "Microsoft.VisualStudioCode",
			Version:          "1.90.0",
			AvailableVersion: "1.91.0",
		},
	}

	nameLen, idLen, verLen := getPackageColumnWidths(updates, 10, 10, 5)

	if nameLen != 18 { // len("Visual Studio Code") == 18
		t.Errorf("expected nameLen=18, got %d", nameLen)
	}

	if idLen != 26 { // len("Microsoft.VisualStudioCode") == 26
		t.Errorf("expected idLen=26, got %d", idLen)
	}

	if verLen != 8 { // len("2.55.0.2") == 8
		t.Errorf("expected verLen=8, got %d", verLen)
	}
}

func TestGetDefaultPackageColumnWidths(t *testing.T) {
	updates := []winget.PackageUpdate{
		{
			Name:             "Git",
			Id:               "Git.Git",
			Version:          "2.5",
			AvailableVersion: "2.6",
		},
	}

	nameLen, idLen, verLen := getDefaultPackageColumnWidths(updates)

	if nameLen != DefaultMinNameWidth {
		t.Errorf("expected nameLen=%d, got %d", DefaultMinNameWidth, nameLen)
	}

	if idLen != DefaultMinIDWidth {
		t.Errorf("expected idLen=%d, got %d", DefaultMinIDWidth, idLen)
	}

	if verLen != DefaultMinVersionWidth {
		t.Errorf("expected verLen=%d, got %d", DefaultMinVersionWidth, verLen)
	}
}

func TestFormatUpdateRow(t *testing.T) {
	pkg := winget.PackageUpdate{
		Name:             "Git",
		Id:               "Git.Git",
		Version:          "2.55.0.2",
		AvailableVersion: "2.55.0.3",
	}

	formatted := formatUpdateRow(pkg, 10, 10, 8)
	expected := "Git          Git.Git      2.55.0.2 -> 2.55.0.3"

	if formatted != expected {
		t.Errorf("expected %q, got %q", expected, formatted)
	}
}
