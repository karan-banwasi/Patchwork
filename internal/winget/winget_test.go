package winget

import (
	"testing"
)

func TestParseWingetOutput_Standard(t *testing.T) {
	sampleOutput := `Name                                      Id                                     Version       Available     Source
------------------------------------------------------------------------------------------------------------------
Git                                       Git.Git                                2.55.0.2      2.55.0.3      winget
Visual Studio Code                        Microsoft.VisualStudioCode             1.90.0        1.91.0        winget
`

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	if updates[0].Name != "Git" || updates[0].Id != "Git.Git" || updates[0].Version != "2.55.0.2" || updates[0].AvailableVersion != "2.55.0.3" || updates[0].Source != "winget" {
		t.Errorf("unexpected package 0 parse: %+v", updates[0])
	}

	if updates[1].Name != "Visual Studio Code" || updates[1].Id != "Microsoft.VisualStudioCode" || updates[1].Version != "1.90.0" || updates[1].AvailableVersion != "1.91.0" {
		t.Errorf("unexpected package 1 parse: %+v", updates[1])
	}
}

func TestParseWingetOutput_WithAnsiAndSpinnerPrefix(t *testing.T) {
	sampleOutput := "\x1b[0m\x1b[2K\r/ Name                                      Id                                     Version       Available     Source\n" +
		"------------------------------------------------------------------------------------------------------------------\n" +
		"Oh My Posh                                JanDeDobbeleer.OhMyPosh                29.26.0.0     29.31.1       winget\n"

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "Oh My Posh" || updates[0].Id != "JanDeDobbeleer.OhMyPosh" || updates[0].Version != "29.26.0.0" || updates[0].AvailableVersion != "29.31.1" {
		t.Errorf("unexpected package parse result: %+v", updates[0])
	}
}

func TestParseWingetOutput_NoUpdates(t *testing.T) {
	sampleOutput := "No installed package found matching input criteria."

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestParseWingetOutput_Empty(t *testing.T) {
	updates, err := parseWingetOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}
