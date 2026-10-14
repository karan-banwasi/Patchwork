package pm

import (
	"context"
	"errors"
	"testing"
)

type mockManager struct {
	name      string
	available bool
	updates   []PackageUpdate
	err       error
}

func (m *mockManager) Name() string { return m.name }
func (m *mockManager) IsAvailable(ctx context.Context) bool {
	return m.available
}
func (m *mockManager) GetAvailableUpdates(ctx context.Context) ([]PackageUpdate, error) {
	return m.updates, m.err
}
func (m *mockManager) UpgradePackage(ctx context.Context, id string) error { return m.err }
func (m *mockManager) UpgradeAll(ctx context.Context) error                 { return m.err }

func TestRegistry_ActiveManagers(t *testing.T) {
	m1 := &mockManager{name: "winget", available: true}
	m2 := &mockManager{name: "scoop", available: false}
	m3 := &mockManager{name: "choco", available: true}

	reg := NewRegistry(m1, m2, m3)
	active := reg.ActiveManagers(context.Background())

	if len(active) != 2 {
		t.Fatalf("expected 2 active managers, got %d", len(active))
	}
	if active[0].Name() != "winget" || active[1].Name() != "choco" {
		t.Errorf("unexpected active managers: %v, %v", active[0].Name(), active[1].Name())
	}
}

func TestRegistry_FetchAllUpdates(t *testing.T) {
	m1 := &mockManager{
		name:      "winget",
		available: true,
		updates: []PackageUpdate{
			{Name: "Git", Id: "Git.Git", Version: "2.55.0.2", ManagerName: "winget"},
		},
	}
	m2 := &mockManager{
		name:      "choco",
		available: true,
		updates: []PackageUpdate{
			{Name: "Node.js", Id: "nodejs", Version: "20.0.0", ManagerName: "choco"},
		},
	}

	reg := NewRegistry(m1, m2)
	updates, err := reg.FetchAllUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	ids := map[string]bool{updates[0].Id: true, updates[1].Id: true}
	if !ids["Git.Git"] || !ids["nodejs"] {
		t.Errorf("unexpected updates list: %+v", updates)
	}
}

func TestRegistry_GetManager(t *testing.T) {
	m1 := &mockManager{name: "winget", available: true}
	m2 := &mockManager{name: "scoop", available: true}
	m3 := &mockManager{name: "choco", available: true}

	reg := NewRegistry(m1, m2, m3)

	// Exact match
	mgr, found := reg.GetManager("winget")
	if !found || mgr.Name() != "winget" {
		t.Errorf("expected to find winget manager, got found=%v, mgr=%v", found, mgr)
	}

	// Case-insensitive match
	mgrUpper, foundUpper := reg.GetManager("Winget")
	if !foundUpper || mgrUpper.Name() != "winget" {
		t.Errorf("expected to find winget manager via 'Winget', got found=%v, mgr=%v", foundUpper, mgrUpper)
	}

	// Alias match: chocolatey -> choco
	mgrChoco, foundChoco := reg.GetManager("chocolatey")
	if !foundChoco || mgrChoco.Name() != "choco" {
		t.Errorf("expected to find choco manager via 'chocolatey' alias, got found=%v, mgr=%v", foundChoco, mgrChoco)
	}

	_, foundMissing := reg.GetManager("unknown")
	if foundMissing {
		t.Errorf("expected found=false for unknown manager")
	}
}

func TestRegistry_FetchAllUpdates_Error(t *testing.T) {
	m1 := &mockManager{
		name:      "winget",
		available: true,
		err:       errors.New("command failed"),
	}

	reg := NewRegistry(m1)
	_, err := reg.FetchAllUpdates(context.Background())
	if err == nil {
		t.Fatalf("expected error when all managers fail, got nil")
	}
}

func TestRegistry_FetchAllUpdates_PartialFailureWithZeroUpdates(t *testing.T) {
	m1 := &mockManager{
		name:      "winget",
		available: true,
		updates:   []PackageUpdate{}, // succeeded with 0 updates
		err:       nil,
	}
	m2 := &mockManager{
		name:      "choco",
		available: true,
		err:       errors.New("elevation required"),
	}

	reg := NewRegistry(m1, m2)
	updates, err := reg.FetchAllUpdates(context.Background())
	if err != nil {
		t.Fatalf("expected no error when at least one manager succeeds, got: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestRegistry_FetchAllUpdates_Deduplication(t *testing.T) {
	m1 := &mockManager{
		name:      "winget",
		available: true,
		updates: []PackageUpdate{
			{Name: "Git", Id: "Git.Git", Version: "2.40.0", ManagerName: "winget"},
			{Name: "Git", Id: "Git.Git", Version: "2.40.0", ManagerName: "winget"}, // duplicate
		},
		err: nil,
	}

	reg := NewRegistry(m1)
	updates, err := reg.FetchAllUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 deduplicated update, got %d", len(updates))
	}
}


