package pm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type mockManager struct {
	name      string
	aliases   []string
	available bool
	updates   []PackageUpdate
	err       error
	block     bool
}

func (m *mockManager) Name() string      { return m.name }
func (m *mockManager) Aliases() []string { return m.aliases }
func (m *mockManager) IsAvailable(ctx context.Context) bool {
	return m.available
}
func (m *mockManager) GetAvailableUpdates(ctx context.Context) ([]PackageUpdate, error) {
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return m.updates, m.err
}
func (m *mockManager) UpgradePackage(ctx context.Context, id string) error {
	if err := m.ValidateID(id); err != nil {
		return err
	}
	return m.err
}
func (m *mockManager) ValidateID(id string) error {
	return ValidateGenericID(id)
}

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
	m3 := &mockManager{name: "choco", aliases: []string{"chocolatey"}, available: true}

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
	updates, warnings, err := reg.FetchAllUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("expected no error when at least one manager succeeds, got: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "elevation required") {
		t.Errorf("expected 1 warning containing 'elevation required', got %v", warnings)
	}
}

func TestRegistry_FetchAllUpdates_Timeout(t *testing.T) {
	fast := &mockManager{
		name:      "winget",
		available: true,
		updates: []PackageUpdate{
			{Name: "Git", Id: "Git.Git", Version: "2.40.0", AvailableVersion: "2.41.0", ManagerName: "winget"},
		},
	}
	hung := &mockManager{
		name:      "psresource",
		available: true,
		block:     true,
	}

	reg := NewRegistry(fast, hung)
	reg.FetchTimeout = 50 * time.Millisecond

	start := time.Now()
	updates, warnings, err := reg.FetchAllUpdatesWithWarnings(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected partial success when one manager times out, got error: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected timeout around 50ms, took %v", elapsed)
	}
	if len(updates) != 1 || updates[0].Id != "Git.Git" {
		t.Errorf("expected Git.Git update from non-hung manager, got: %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "timed out") {
		t.Errorf("expected timeout warning for psresource, got: %v", warnings)
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

func TestValidateID_PerManager(t *testing.T) {
	maliciousPayloads := []string{
		"",
		"-f",
		"--git=https://attacker/repo",
		"--source=https://evil.example/feed",
		"Microsoft.PowerShell&calc",
		"pkg|calc",
		"pkg;calc",
		"pkg`calc`",
		"pkg$(calc)",
		"%PATH%",
		"!VAR!",
		"pkg with spaces",
		"pkg\nnewline",
		"pkg\rcarriage",
		"pkg'quote",
		"pkg‘curlyquote",
		"pkg’curlyquote",
		"pkg\"doublequote",
		"../traversal",
	}

	validators := map[string]func(string) error{
		"winget":     ValidateWingetID,
		"scoop":      ValidateScoopID,
		"choco":      ValidateChocoID,
		"npm":        ValidateNpmID,
		"cargo":      ValidateCargoID,
		"uv":         ValidateUvID,
		"dotnet":     ValidateDotnetID,
		"psresource": ValidatePSResourceID,
		"generic":    ValidateGenericID,
	}

	for mgrName, fn := range validators {
		for _, bad := range maliciousPayloads {
			if err := fn(bad); err == nil {
				t.Errorf("%s validator accepted malicious payload %q", mgrName, bad)
			}
		}
	}

	validCases := []struct {
		mgr string
		id  string
	}{
		{"winget", "Git.Git"},
		{"winget", "Microsoft.PowerShell"},
		{"winget", "Notepad++.Notepad++"},
		{"winget", "7zip.7zip"},
		{"scoop", "git"},
		{"scoop", "extras/vscode"},
		{"scoop", "main/7zip"},
		{"choco", "git.install"},
		{"choco", "nodejs-lts"},
		{"npm", "typescript"},
		{"npm", "@angular/cli"},
		{"npm", "@types/node"},
		{"cargo", "ripgrep"},
		{"cargo", "cargo-update"},
		{"uv", "ruff"},
		{"uv", "yt-dlp"},
		{"dotnet", "dotnet-ef"},
		{"dotnet", "Microsoft.DotNet.Watcher.Tool"},
		{"psresource", "PSReadLine"},
		{"psresource", "Microsoft.PowerShell.PSResourceGet"},
	}

	for _, tc := range validCases {
		if err := validators[tc.mgr](tc.id); err != nil {
			t.Errorf("%s validator rejected valid package ID %q: %v", tc.mgr, tc.id, err)
		}
	}
}

func TestQuoteWindowsArg(t *testing.T) {
	got := QuoteWindowsArg(`C:\Path With Spaces\backslash\`)
	want := `"C:\Path With Spaces\backslash\\"`
	if got != want {
		t.Errorf("QuoteWindowsArg got %s, want %s", got, want)
	}

	gotQuote := QuoteWindowsArg(`arg\"with"quotes`)
	wantQuote := `"arg\\\"with\"quotes"`
	if gotQuote != wantQuote {
		t.Errorf("QuoteWindowsArg got %s, want %s", gotQuote, wantQuote)
	}
}
