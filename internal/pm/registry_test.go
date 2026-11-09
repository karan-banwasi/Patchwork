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
		"brew":       ValidateBrewID,
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
		{"brew", "wget"},
		{"brew", "node@20"},
		{"brew", "homebrew/cask/visual-studio-code"},
		{"brew", "gtk+3"},
		{"brew", "hashicorp/tap/terraform"},
		{"brew", "font-fira-code"},
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

type mockTimeoutManager struct {
	mockManager
	timeout time.Duration
}

func (m *mockTimeoutManager) FetchTimeout() time.Duration {
	return m.timeout
}

type mockConfigurableManager struct {
	mockManager
	opts UpgradeOptions
}

func (m *mockConfigurableManager) ConfigureOptions(opts UpgradeOptions) {
	m.opts = opts
}

type mockDeferringManager struct {
	mockManager
	deferIDs map[string]bool
}

func (m *mockDeferringManager) ShouldDefer(id string) bool {
	return m.deferIDs[id]
}

type mockRefreshManager struct {
	mockManager
	refreshErr error
}

func (m *mockRefreshManager) Refresh(ctx context.Context) error {
	return m.refreshErr
}

type mockWarningManager struct {
	mockManager
	warnings []error
}

func (m *mockWarningManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]PackageUpdate, []error, error) {
	return m.updates, m.warnings, m.err
}

func TestNewRegistry_Defaults(t *testing.T) {
	reg := NewRegistry()
	if reg.FetchTimeout != 0 {
		t.Fatalf("expected NewRegistry FetchTimeout to be 0, got %v", reg.FetchTimeout)
	}
}

func TestRegistry_ManagerTimeout(t *testing.T) {
	baseMgr := &mockManager{name: "base"}
	customMgr := &mockTimeoutManager{
		mockManager: mockManager{name: "custom"},
		timeout:     30 * time.Second,
	}

	reg := NewRegistry(baseMgr, customMgr)

	// 1. Unspecified reg.FetchTimeout (0) -> baseMgr gets DefaultFetchTimeout (2m)
	if d := reg.ManagerTimeout(baseMgr); d != DefaultFetchTimeout {
		t.Errorf("expected baseMgr timeout to be %v, got %v", DefaultFetchTimeout, d)
	}

	// 2. Unspecified reg.FetchTimeout (0) -> customMgr gets 30s
	if d := reg.ManagerTimeout(customMgr); d != 30*time.Second {
		t.Errorf("expected customMgr timeout to be 30s, got %v", d)
	}

	// 3. Explicit reg.FetchTimeout (10s) overrides customMgr
	reg.FetchTimeout = 10 * time.Second
	if d := reg.ManagerTimeout(customMgr); d != 10*time.Second {
		t.Errorf("expected customMgr timeout to be overridden to 10s, got %v", d)
	}
	if d := reg.ManagerTimeout(baseMgr); d != 10*time.Second {
		t.Errorf("expected baseMgr timeout to be overridden to 10s, got %v", d)
	}
}

func TestRegistry_ConfigureOptions(t *testing.T) {
	m1 := &mockConfigurableManager{mockManager: mockManager{name: "cfg1"}}
	m2 := &mockManager{name: "noncfg"}

	reg := NewRegistry(m1, m2)
	opts := UpgradeOptions{
		AcceptAgreements: true,
		TrustRepository:  true,
	}
	reg.ConfigureOptions(opts)

	if !m1.opts.AcceptAgreements || !m1.opts.TrustRepository {
		t.Errorf("expected m1 to receive options %+v, got %+v", opts, m1.opts)
	}
}

func TestRegistry_ShouldDefer(t *testing.T) {
	m1 := &mockDeferringManager{
		mockManager: mockManager{name: "winget"},
		deferIDs: map[string]bool{
			"Microsoft.PowerShell": true,
		},
	}
	m2 := &mockManager{name: "choco"}

	reg := NewRegistry(m1, m2)

	// Deferring manager with matching ID
	if !reg.ShouldDefer(PackageUpdate{ManagerName: "winget", Id: "Microsoft.PowerShell"}) {
		t.Errorf("expected winget:Microsoft.PowerShell to defer")
	}

	// Deferring manager with non-matching ID
	if reg.ShouldDefer(PackageUpdate{ManagerName: "winget", Id: "Git.Git"}) {
		t.Errorf("expected winget:Git.Git not to defer")
	}

	// Non-deferring manager even with PowerShell ID (no hardcoded IDs in pm.Registry!)
	if reg.ShouldDefer(PackageUpdate{ManagerName: "choco", Id: "powershell"}) {
		t.Errorf("expected choco:powershell not to defer without manager Deferrer support")
	}

	// Unknown manager
	if reg.ShouldDefer(PackageUpdate{ManagerName: "unknown", Id: "anything"}) {
		t.Errorf("expected unknown manager not to defer")
	}

	// Nil registry
	var nilReg *Registry
	if nilReg.ShouldDefer(PackageUpdate{ManagerName: "winget", Id: "Microsoft.PowerShell"}) {
		t.Errorf("expected nil registry not to defer")
	}
}

func TestRegistry_FetchUpdates_FilterAndErrors(t *testing.T) {
	m1 := &mockManager{
		name:      "winget",
		available: true,
		updates: []PackageUpdate{
			{Name: "Git", Id: "Git.Git", ManagerName: "winget"},
		},
	}
	m2 := &mockManager{
		name:      "scoop",
		available: false,
	}

	reg := NewRegistry(m1, m2)

	// Filter matching available manager
	updates, warnings, err := reg.FetchUpdates(context.Background(), "winget")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "Git.Git" {
		t.Errorf("expected Git.Git update, got %+v", updates)
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", warnings)
	}

	// Filter with unavailable manager
	_, _, err = reg.FetchUpdates(context.Background(), "scoop")
	if err == nil || !strings.Contains(err.Error(), "not installed or available") {
		t.Errorf("expected unavailable error for scoop, got: %v", err)
	}

	// Filter with unknown manager
	_, _, err = reg.FetchUpdates(context.Background(), "unknown")
	if err == nil || !strings.Contains(err.Error(), "unknown package manager") {
		t.Errorf("expected unknown error for unknown, got: %v", err)
	}
}

func TestRegistry_FetchUpdates_RefreshWarning(t *testing.T) {
	m1 := &mockRefreshManager{
		mockManager: mockManager{
			name:      "scoop",
			available: true,
			updates: []PackageUpdate{
				{Name: "7zip", Id: "7zip", ManagerName: "scoop"},
			},
		},
		refreshErr: errors.New("git lock failed"),
	}

	reg := NewRegistry(m1)

	// SkipRefresh = false -> refresh error surfaces as warning, updates still returned
	updates, warnings, err := reg.FetchUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "7zip" {
		t.Errorf("expected 7zip update despite refresh warning, got %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "refresh failed: git lock failed") {
		t.Errorf("expected refresh warning, got %v", warnings)
	}

	// SkipRefresh = true -> refresh skipped, no warning
	reg.SkipRefresh = true
	updates2, warnings2, err2 := reg.FetchUpdates(context.Background())
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if len(warnings2) != 0 {
		t.Errorf("expected 0 warnings when SkipRefresh=true, got %v", warnings2)
	}
	if len(updates2) != 1 {
		t.Errorf("expected 1 update, got %d", len(updates2))
	}
}

func TestRegistry_FetchUpdates_WarningFetcher(t *testing.T) {
	m1 := &mockWarningManager{
		mockManager: mockManager{
			name:      "cargo",
			available: true,
			updates: []PackageUpdate{
				{Name: "ripgrep", Id: "ripgrep", ManagerName: "cargo"},
			},
		},
		warnings: []error{errors.New("[cargo] 1 of 2 crates could not be checked; network error")},
	}

	reg := NewRegistry(m1)
	updates, warnings, err := reg.FetchUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "ripgrep" {
		t.Errorf("expected ripgrep update, got %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "1 of 2 crates could not be checked") {
		t.Errorf("expected lookup warning, got %v", warnings)
	}
}

func TestRegistry_FetchUpdates_DowngradeFiltered(t *testing.T) {
	m1 := &mockManager{
		name:      "generic",
		available: true,
		updates: []PackageUpdate{
			{Name: "real-upgrade", Id: "real-upgrade", Version: "1.0.0", AvailableVersion: "1.1.0", ManagerName: "generic"},
			{Name: "downgrade-pkg", Id: "downgrade-pkg", Version: "2.0.0", AvailableVersion: "1.9.0", ManagerName: "generic"},
			{Name: "same-ver-pkg", Id: "same-ver-pkg", Version: "3.0.0", AvailableVersion: "3.0.0", ManagerName: "generic"},
		},
	}

	reg := NewRegistry(m1)
	updates, _, err := reg.FetchUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update after filtering downgrades/same versions, got %d: %+v", len(updates), updates)
	}
	if updates[0].Id != "real-upgrade" {
		t.Errorf("expected real-upgrade, got %s", updates[0].Id)
	}
}

type mockNetworkManager struct {
	mockManager
	requiresNetwork bool
	offline         bool
}

func (m *mockNetworkManager) RequiresNetwork() bool   { return m.requiresNetwork }
func (m *mockNetworkManager) SetOffline(offline bool) { m.offline = offline }

func TestRegistry_OfflineMode(t *testing.T) {
	mLocal := &mockManager{
		name:      "winget",
		available: true,
		updates: []PackageUpdate{
			{Name: "git", Id: "Git.Git", Version: "2.40.0", AvailableVersion: "2.41.0", ManagerName: "winget"},
		},
	}
	mNet := &mockNetworkManager{
		mockManager: mockManager{
			name:      "cargo",
			available: true,
			updates: []PackageUpdate{
				{Name: "ripgrep", Id: "ripgrep", Version: "13.0.0", AvailableVersion: "14.0.0", ManagerName: "cargo"},
			},
		},
		requiresNetwork: true,
	}

	reg := NewRegistry(mLocal, mNet)
	reg.SetOffline(true)

	if !reg.Offline || !reg.SkipRefresh || !mNet.offline {
		t.Errorf("expected offline mode and SkipRefresh=true to propagate")
	}

	updates, warnings, err := reg.FetchUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(updates) != 1 || updates[0].Id != "Git.Git" {
		t.Errorf("expected only local winget update, got %+v", updates)
	}

	foundWarning := false
	for _, w := range warnings {
		if strings.Contains(w.Error(), "[cargo] offline mode") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected cargo offline skip warning, got: %v", warnings)
	}
}
