package scoop

import (
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestScoopManager_Name(t *testing.T) {
	mgr := NewScoopManager()
	if mgr.Name() != "scoop" {
		t.Errorf("expected manager name 'scoop', got %q", mgr.Name())
	}
	var _ pm.Manager = mgr // compile-time interface check
}

func TestParseScoopOutput_Standard(t *testing.T) {
	sampleOutput := `Scoop is up to date.
Updates are available for:

Name      Installed Version Latest Version Missing dependencies
----      ----------------- -------------- --------------------
git       2.40.0            2.41.0
neovim    0.8.3             0.9.0
`

	updates, err := parseScoopOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	if updates[0].Name != "git" || updates[0].Id != "git" || updates[0].Version != "2.40.0" || updates[0].AvailableVersion != "2.41.0" || updates[0].ManagerName != "scoop" {
		t.Errorf("unexpected package 0 parse: %+v", updates[0])
	}

	if updates[1].Name != "neovim" || updates[1].Id != "neovim" || updates[1].Version != "0.8.3" || updates[1].AvailableVersion != "0.9.0" {
		t.Errorf("unexpected package 1 parse: %+v", updates[1])
	}
}

func TestParseScoopOutput_WithAnsi(t *testing.T) {
	sampleOutput := "\x1b[0m\x1b[2KScoop is up to date.\n" +
		"Name      Installed Version Latest Version\n" +
		"----      ----------------- --------------\n" +
		"vscode    1.77.3            1.78.0        \n"

	updates, err := parseScoopOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}

	if updates[0].Name != "vscode" || updates[0].Id != "vscode" || updates[0].Version != "1.77.3" || updates[0].AvailableVersion != "1.78.0" {
		t.Errorf("unexpected package parse: %+v", updates[0])
	}
}

func TestParseScoopOutput_NoUpdates(t *testing.T) {
	sampleOutput := "Everything is up to date."

	updates, err := parseScoopOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestParseScoopOutput_Empty(t *testing.T) {
	updates, err := parseScoopOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestParseScoopOutput_LineFormat(t *testing.T) {
	sampleOutput := `
Updates are available for:
  curl: 7.88.1 -> 8.0.1
  7zip: 22.01 -> 23.01
`

	updates, err := parseScoopOutput(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	if updates[0].Name != "curl" || updates[0].Version != "7.88.1" || updates[0].AvailableVersion != "8.0.1" {
		t.Errorf("unexpected package 0 parse: %+v", updates[0])
	}
}
