package history

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHistoryPath_EnvOverrideAndDefault(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", custom)
	if got := HistoryPath(); got != custom {
		t.Errorf("HistoryPath() = %q, want %q", got, custom)
	}

	t.Setenv("PATCHWORK_HISTORY", "")
	gotDefault := HistoryPath()
	if !strings.HasSuffix(gotDefault, filepath.Join("Patchwork", "history.jsonl")) &&
		!strings.HasSuffix(gotDefault, "patchwork-history.jsonl") {
		t.Errorf("unexpected default HistoryPath(): %q", gotDefault)
	}
}

func TestLoadAll_MissingFile(t *testing.T) {
	t.Setenv("PATCHWORK_HISTORY", filepath.Join(t.TempDir(), "nonexistent.jsonl"))
	entries, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() unexpected error on missing file: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestAppendLoadAllAndClear(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "nested", "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	t1 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 16, 11, 0, 0, 0, time.UTC)

	e1 := Entry{
		SchemaVersion: SchemaVersion,
		RunID:         "run-1",
		Timestamp:     t1,
		ManagerFilter: "npm",
		Items: []Item{
			{Name: "typescript", ID: "typescript", Manager: "npm", FromVersion: "5.4.0", ToVersion: "5.5.0", Status: "Succeeded", DurationMs: 1200},
		},
		Succeeded: 1,
	}
	e2 := Entry{
		RunID:     "run-2",
		Timestamp: t2,
		Items: []Item{
			{Name: "Git", ID: "Git.Git", Manager: "winget", FromVersion: "2.44.0", ToVersion: "2.45.0", Status: "Succeeded", DurationMs: 3400},
			{Name: "curl", ID: "curl", Manager: "scoop", FromVersion: "8.6.0", ToVersion: "8.7.0", Status: "Failed", DurationMs: 500, Error: "network error"},
		},
		Succeeded: 1,
		Failed:    1,
	}

	if err := Append(e1); err != nil {
		t.Fatalf("Append(e1) failed: %v", err)
	}
	if err := Append(e2); err != nil {
		t.Fatalf("Append(e2) failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(histFile)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected file perm 0600, got %o", perm)
		}
		dirInfo, err := os.Stat(filepath.Dir(histFile))
		if err != nil {
			t.Fatalf("Stat dir failed: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("expected dir perm 0700, got %o", perm)
		}
	}

	entries, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].RunID != "run-1" || entries[1].RunID != "run-2" {
		t.Errorf("unexpected run IDs: %+v", entries)
	}
	if entries[1].SchemaVersion != SchemaVersion {
		t.Errorf("expected default schema version %q, got %q", SchemaVersion, entries[1].SchemaVersion)
	}

	if err := Clear(); err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}
	if _, err := os.Stat(histFile); !os.IsNotExist(err) {
		t.Errorf("expected history file to be removed after Clear()")
	}
	// Clearing again should be idempotent
	if err := Clear(); err != nil {
		t.Errorf("second Clear() failed: %v", err)
	}
}

func TestLoadAll_SkipsMalformedAndBlankLines(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	raw := "\n" +
		"{not-valid-json\n" +
		`{"schema_version":"1.0.0","run_id":"run-ok","timestamp":"2026-11-16T12:00:00Z","items":[{"id":"ripgrep","manager":"cargo","from_version":"14.0.0","to_version":"14.1.0","status":"Succeeded"}],"succeeded":1}` + "\n" +
		"   \n" +
		"{}\n"

	if err := os.WriteFile(histFile, []byte(raw), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	entries, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() failed: %v", err)
	}
	if len(entries) != 1 || entries[0].RunID != "run-ok" {
		t.Fatalf("expected 1 valid entry 'run-ok', got %+v", entries)
	}
}

func TestFindLatestForPackageAndFindEntry(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	_ = Append(Entry{
		RunID:     "run-old",
		Timestamp: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
		Items: []Item{
			{Name: "Git", ID: "Git.Git", Manager: "winget", FromVersion: "2.43.0", ToVersion: "2.44.0", Status: "Succeeded"},
			{Name: "git", ID: "git", Manager: "scoop", FromVersion: "2.43.0", ToVersion: "2.44.0", Status: "Succeeded"},
		},
		Succeeded: 2,
	})
	_ = Append(Entry{
		RunID:     "run-mid",
		Timestamp: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC),
		Items: []Item{
			{Name: "Git", ID: "Git.Git", Manager: "winget", FromVersion: "2.44.0", ToVersion: "2.45.0", Status: "Succeeded"},
		},
		Succeeded: 1,
	})
	_ = Append(Entry{
		RunID:     "run-fail",
		Timestamp: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
		Items: []Item{
			// Latest attempt failed, so FindLatestForPackage must skip it and return run-mid
			{Name: "Git", ID: "Git.Git", Manager: "winget", FromVersion: "2.45.0", ToVersion: "2.46.0", Status: "Failed", Error: "checksum mismatch"},
		},
		Failed: 1,
	})

	item, entry, err := FindLatestForPackage("", "git.git")
	if err != nil {
		t.Fatalf("FindLatestForPackage failed: %v", err)
	}
	if item == nil || entry == nil {
		t.Fatalf("expected item and entry, got nil")
	}
	if entry.RunID != "run-mid" || item.FromVersion != "2.44.0" || item.ToVersion != "2.45.0" {
		t.Errorf("unexpected result: entry=%s item=%+v", entry.RunID, *item)
	}

	// Manager-scoped lookup
	scoopItem, scoopEntry, err := FindLatestForPackage("scoop", "git")
	if err != nil {
		t.Fatalf("FindLatestForPackage(scoop) failed: %v", err)
	}
	if scoopItem == nil || scoopEntry == nil || scoopEntry.RunID != "run-old" || scoopItem.Manager != "scoop" {
		t.Errorf("unexpected scoop lookup result: entry=%v item=%v", scoopEntry, scoopItem)
	}

	// Missing package
	missingItem, missingEntry, err := FindLatestForPackage("", "does.not.exist")
	if err != nil {
		t.Fatalf("unexpected error for missing package: %v", err)
	}
	if missingItem != nil || missingEntry != nil {
		t.Errorf("expected nil for missing package, got %+v / %+v", missingItem, missingEntry)
	}

	// FindEntry
	found, err := FindEntry("RUN-MID")
	if err != nil {
		t.Fatalf("FindEntry failed: %v", err)
	}
	if found == nil || found.RunID != "run-mid" {
		t.Errorf("expected run-mid, got %+v", found)
	}

	notFound, err := FindEntry("nonexistent")
	if err != nil {
		t.Fatalf("FindEntry unexpected error: %v", err)
	}
	if notFound != nil {
		t.Errorf("expected nil for nonexistent run, got %+v", notFound)
	}
}
