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
	if updates[0].Id != "Git.Git" || updates[1].Id != "nodejs" {
		t.Errorf("unexpected updates list: %+v", updates)
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
