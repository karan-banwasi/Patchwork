package winget

import (
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestWingetManager_Name(t *testing.T) {
	mgr := NewWingetManager()
	if mgr.Name() != "winget" {
		t.Errorf("expected manager name 'winget', got %q", mgr.Name())
	}
	var _ pm.Manager = mgr // compile-time interface check
}

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

	if updates[0].Name != "Git" || updates[0].Id != "Git.Git" || updates[0].Version != "2.55.0.2" || updates[0].AvailableVersion != "2.55.0.3" || updates[0].Source != "winget" || updates[0].ManagerName != "winget" {
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

func TestParseWingetOutput_LeadingSpacesHeader(t *testing.T) {
	sampleOutput := "    Name                                  Id                                     Version       Available     Source\n" +
		"------------------------------------------------------------------------------------------------------------------\n" +
		"Docker Desktop                            Docker.DockerDesktop                   4.30.0        4.31.0        winget\n"

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "Docker Desktop" || updates[0].Id != "Docker.DockerDesktop" || updates[0].Version != "4.30.0" || updates[0].AvailableVersion != "4.31.0" {
		t.Errorf("unexpected package parse result: %+v", updates[0])
	}
}

func TestParseWingetOutput_UTF8(t *testing.T) {
	sampleOutput := "Name                                      Id                                     Version       Available     Source\n" +
		"------------------------------------------------------------------------------------------------------------------\n" +
		"7-Zip® Archiver                            7zip.7zip                              23.01         24.05         winget\n"

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "7-Zip® Archiver" || updates[0].Id != "7zip.7zip" || updates[0].Version != "23.01" || updates[0].AvailableVersion != "24.05" {
		t.Errorf("unexpected package parse result: %+v", updates[0])
	}
}

func TestParseWingetOutput_LocalizedHeader(t *testing.T) {
	// Sample German winget output
	sampleOutput := "Name                                      ID                                     Version       Verfügbar     Quelle\n" +
		"------------------------------------------------------------------------------------------------------------------\n" +
		"VLC media player                          VideoLAN.VLC                           3.0.18        3.0.20        winget\n"

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "VLC media player" || updates[0].Id != "VideoLAN.VLC" || updates[0].Version != "3.0.18" || updates[0].AvailableVersion != "3.0.20" {
		t.Errorf("unexpected package parse result: %+v", updates[0])
	}
}

func TestParseWingetOutput_UppercaseID(t *testing.T) {
	sampleOutput := "Name                                      ID                                     Version       Available     Source\n" +
		"------------------------------------------------------------------------------------------------------------------\n" +
		"Notepad++                                 Notepad++.Notepad++                    8.5.8         8.6.0         winget\n"

	updates, err := parseWingetOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "Notepad++" || updates[0].Id != "Notepad++.Notepad++" {
		t.Errorf("unexpected package parse result: %+v", updates[0])
	}
}

func TestShouldOmitSilent(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"Microsoft.PowerShell", true},
		{"Microsoft.PowerShell.Preview", true},
		{"microsoft.powershell", true},
		{"MICROSOFT.POWERSHELL", true},
		{"Microsoft.WindowsTerminal", true},
		{"Microsoft.WindowsTerminal.Preview", true},
		{"Git.Git", false},
		{"JanDeDobbeleer.OhMyPosh", false},
		{"Docker.DockerDesktop", false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			actual := shouldOmitSilent(tc.id)
			if actual != tc.expected {
				t.Errorf("shouldOmitSilent(%q) = %v, expected %v", tc.id, actual, tc.expected)
			}
		})
	}
}

func TestBuildUpgradeArgs(t *testing.T) {
	psArgs := buildUpgradeArgs("Microsoft.PowerShell")
	for _, arg := range psArgs {
		if arg == "--silent" {
			t.Errorf("expected --silent to be omitted for Microsoft.PowerShell, got args: %v", psArgs)
		}
	}

	gitArgs := buildUpgradeArgs("Git.Git")
	hasSilent := false
	for _, arg := range gitArgs {
		if arg == "--silent" {
			hasSilent = true
			break
		}
	}
	if !hasSilent {
		t.Errorf("expected --silent to be included for Git.Git, got args: %v", gitArgs)
	}
}



