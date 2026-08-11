package choco

import (
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestChocoManager_Name(t *testing.T) {
	mgr := NewChocoManager()
	if mgr.Name() != "choco" {
		t.Errorf("expected manager name 'choco', got %q", mgr.Name())
	}
	var _ pm.Manager = mgr // compile-time interface check
}

func TestParseChocoOutput_LimitOutput(t *testing.T) {
	sampleOutput := `Chocolatey v1.4.0
git.install|2.40.0|2.41.0|false
nodejs|20.0.0|20.1.0|false
Chocolatey has determined 2 package(s) are outdated.`

	updates, err := parseChocoOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	if updates[0].Name != "git.install" || updates[0].Id != "git.install" || updates[0].Version != "2.40.0" || updates[0].AvailableVersion != "2.41.0" || updates[0].ManagerName != "choco" {
		t.Errorf("unexpected package 0 parse: %+v", updates[0])
	}

	if updates[1].Name != "nodejs" || updates[1].Id != "nodejs" || updates[1].Version != "20.0.0" || updates[1].AvailableVersion != "20.1.0" {
		t.Errorf("unexpected package 1 parse: %+v", updates[1])
	}
}

func TestParseChocoOutput_FallbackFormat(t *testing.T) {
	sampleOutput := `
Outdated Packages
-----------------
7zip.install 22.01 23.01 false
python3 3.10.0 3.11.0 false
`

	updates, err := parseChocoOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	if updates[0].Name != "7zip.install" || updates[0].Version != "22.01" || updates[0].AvailableVersion != "23.01" {
		t.Errorf("unexpected package 0 parse: %+v", updates[0])
	}
}

func TestParseChocoOutput_NoUpdates(t *testing.T) {
	sampleOutput := "Chocolatey has determined 0 package(s) are outdated."

	updates, err := parseChocoOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestParseChocoOutput_Empty(t *testing.T) {
	updates, err := parseChocoOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}
