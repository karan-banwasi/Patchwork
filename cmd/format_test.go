package cmd

import (
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestGetPackageColumnWidths(t *testing.T) {
	updates := []pm.PackageUpdate{
		{
			Name:             "Git",
			Id:               "Git.Git",
			Version:          "2.55.0.2",
			AvailableVersion: "2.55.0.3",
			ManagerName:      "winget",
		},
		{
			Name:             "Visual Studio Code",
			Id:               "Microsoft.VisualStudioCode",
			Version:          "1.90.0",
			AvailableVersion: "1.91.0",
			ManagerName:      "winget",
		},
	}

	nameLen, idLen, verLen, availLen := getPackageColumnWidths(updates, 10, 10, 5, 9)

	if nameLen != 18 { // len("Visual Studio Code") == 18
		t.Errorf("expected nameLen=18, got %d", nameLen)
	}

	if idLen != 26 { // len("Microsoft.VisualStudioCode") == 26
		t.Errorf("expected idLen=26, got %d", idLen)
	}

	if verLen != 8 { // len("2.55.0.2") == 8
		t.Errorf("expected verLen=8, got %d", verLen)
	}

	if availLen != 9 { // DefaultMinAvailableWidth == 9
		t.Errorf("expected availLen=9, got %d", availLen)
	}
}

func TestGetDefaultPackageColumnWidths(t *testing.T) {
	updates := []pm.PackageUpdate{
		{
			Name:             "Git",
			Id:               "Git.Git",
			Version:          "2.5",
			AvailableVersion: "2.6",
			ManagerName:      "winget",
		},
	}

	nameLen, idLen, verLen, availLen := getDefaultPackageColumnWidths(updates)

	if nameLen != DefaultMinNameWidth {
		t.Errorf("expected nameLen=%d, got %d", DefaultMinNameWidth, nameLen)
	}

	if idLen != DefaultMinIDWidth {
		t.Errorf("expected idLen=%d, got %d", DefaultMinIDWidth, idLen)
	}

	if verLen != DefaultMinVersionWidth {
		t.Errorf("expected verLen=%d, got %d", DefaultMinVersionWidth, verLen)
	}

	if availLen != DefaultMinAvailableWidth {
		t.Errorf("expected availLen=%d, got %d", DefaultMinAvailableWidth, availLen)
	}
}

func TestFormatUpdateRow(t *testing.T) {
	pkg := pm.PackageUpdate{
		Name:             "Git",
		Id:               "Git.Git",
		Version:          "2.55.0.2",
		AvailableVersion: "2.55.0.3",
		ManagerName:      "winget",
	}

	formatted := formatUpdateRow(pkg, 10, 10, 8, 9)
	expected := "Git          Git.Git      2.55.0.2    2.55.0.3    winget"

	if formatted != expected {
		t.Errorf("expected %q, got %q", expected, formatted)
	}
}
