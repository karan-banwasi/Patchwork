package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestMatchExclude(t *testing.T) {
	pkgNpm := pm.PackageUpdate{
		Id:          "@types/node",
		Name:        "@types/node",
		ManagerName: "npm",
	}
	pkgWinget := pm.PackageUpdate{
		Id:          "Microsoft.VisualStudioCode",
		Name:        "Visual Studio Code",
		ManagerName: "winget",
	}

	tests := []struct {
		name     string
		pkg      pm.PackageUpdate
		patterns []string
		wantPat  string
		wantHit  bool
	}{
		{"empty patterns", pkgNpm, nil, "", false},
		{"exact id case-insensitive", pkgWinget, []string{"microsoft.visualstudiocode"}, "microsoft.visualstudiocode", true},
		{"wildcard prefix on name", pkgWinget, []string{"Visual *"}, "Visual *", true},
		{"wildcard on manager:id", pkgWinget, []string{"winget:Microsoft.*"}, "winget:Microsoft.*", true},
		{"manager wildcard matches scoped npm", pkgNpm, []string{"npm:*"}, "npm:*", true},
		{"scoped npm glob", pkgNpm, []string{"@types/*"}, "@types/*", true},
		{"comma separated patterns", pkgWinget, []string{"foo*, *Studio*"}, "*Studio*", true},
		{"question mark glob", pm.PackageUpdate{Id: "git2", ManagerName: "brew"}, []string{"git?"}, "git?", true},
		{"question mark no match", pm.PackageUpdate{Id: "git", ManagerName: "brew"}, []string{"git?"}, "", false},
		{"character class glob", pm.PackageUpdate{Id: "python3", ManagerName: "brew"}, []string{"python[23]"}, "python[23]", true},
		{"non matching manager scope", pkgWinget, []string{"scoop:Microsoft.*"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pat, hit := MatchExclude(tt.pkg, tt.patterns)
			if hit != tt.wantHit || pat != tt.wantPat {
				t.Errorf("MatchExclude() = (%q, %v), want (%q, %v)", pat, hit, tt.wantPat, tt.wantHit)
			}
		})
	}
}

func TestFilterPolicies(t *testing.T) {
	pkgs := []pm.PackageUpdate{
		{Id: "patch-pkg", Name: "Patch", Version: "1.2.3", AvailableVersion: "1.2.4", ManagerName: "npm"},
		{Id: "minor-pkg", Name: "Minor", Version: "1.2.3", AvailableVersion: "1.3.0", ManagerName: "npm"},
		{Id: "major-pkg", Name: "Major", Version: "1.2.3", AvailableVersion: "2.0.0", ManagerName: "npm"},
		{Id: "excluded-pkg", Name: "Excluded", Version: "1.0.0", AvailableVersion: "1.0.1", ManagerName: "cargo"},
	}

	// Patch ceiling + exclude
	allowed, skipped, err := FilterPolicies(pkgs, []string{"cargo:excluded-*"}, "patch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowed) != 1 || allowed[0].Id != "patch-pkg" {
		t.Errorf("expected only patch-pkg allowed, got %+v", allowed)
	}
	if len(skipped) != 3 {
		t.Fatalf("expected 3 skipped, got %d", len(skipped))
	}
	if skipped[0].Status != StatusSkippedLevel || skipped[1].Status != StatusSkippedLevel || skipped[2].Status != StatusSkippedExcluded {
		t.Errorf("unexpected skipped statuses: %+v", skipped)
	}

	// Minor ceiling
	allowedMinor, skippedMinor, err := FilterPolicies(pkgs, nil, "minor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowedMinor) != 3 || len(skippedMinor) != 1 || skippedMinor[0].Package.Id != "major-pkg" {
		t.Errorf("unexpected minor ceiling result: allowed=%d, skipped=%+v", len(allowedMinor), skippedMinor)
	}

	// Invalid level
	if _, _, err := FilterPolicies(pkgs, nil, "bad-level"); err == nil {
		t.Errorf("expected error for invalid level")
	}
}

func TestChangelogURLAndRenderPreview(t *testing.T) {
	cases := []struct {
		pkg  pm.PackageUpdate
		want string
	}{
		{pm.PackageUpdate{Id: "typescript", ManagerName: "npm"}, "https://www.npmjs.com/package/typescript"},
		{pm.PackageUpdate{Id: "@types/node", ManagerName: "npm"}, "https://www.npmjs.com/package/@types/node"},
		{pm.PackageUpdate{Id: "ripgrep", ManagerName: "cargo"}, "https://crates.io/crates/ripgrep"},
		{pm.PackageUpdate{Id: "ruff", ManagerName: "uv"}, "https://pypi.org/project/ruff/"},
		{pm.PackageUpdate{Id: "black", ManagerName: "pipx"}, "https://pypi.org/project/black/"},
		{pm.PackageUpdate{Id: "rails", ManagerName: "gem"}, "https://rubygems.org/gems/rails"},
		{pm.PackageUpdate{Id: "dotnet-ef", ManagerName: "dotnet"}, "https://www.nuget.org/packages/dotnet-ef"},
		{pm.PackageUpdate{Id: "git", Source: "homebrew", ManagerName: "brew"}, "https://formulae.brew.sh/formula/git"},
		{pm.PackageUpdate{Id: "firefox", Source: "cask", ManagerName: "brew"}, "https://formulae.brew.sh/cask/firefox"},
		{pm.PackageUpdate{Id: "jq", ManagerName: "scoop"}, "https://scoop.sh/#/apps?q=jq"},
		{pm.PackageUpdate{Id: "Git.Git", ManagerName: "winget"}, "https://winget.run/pkg/Git/Git"},
		{pm.PackageUpdate{Id: "git", ManagerName: "choco"}, "https://community.chocolatey.org/packages/git"},
		{pm.PackageUpdate{Id: "unknown-pkg", ManagerName: "unknown-mgr"}, ""},
	}

	for _, tc := range cases {
		if got := ChangelogURL(tc.pkg); got != tc.want {
			t.Errorf("ChangelogURL(%+v) = %q, want %q", tc.pkg, got, tc.want)
		}
	}

	var buf bytes.Buffer
	RenderChangelogPreview(&buf, []pm.PackageUpdate{
		{Id: "typescript", Version: "5.3.0", AvailableVersion: "5.4.0", ManagerName: "npm"},
		{Id: "ripgrep", Version: "14.0.0", AvailableVersion: "14.1.0", ManagerName: "cargo"},
	})
	out := buf.String()
	if !strings.Contains(out, "=== Changelog Preview ===") ||
		!strings.Contains(out, "https://www.npmjs.com/package/typescript") ||
		!strings.Contains(out, "https://crates.io/crates/ripgrep") ||
		!strings.Contains(out, "minor") {
		t.Errorf("unexpected changelog preview output:\n%s", out)
	}
}

type parallelTestManager struct {
	name       string
	delay      time.Duration
	deferIDs   map[string]bool
	activeNow  *atomic.Int32
	maxActive  *atomic.Int32
	mu         sync.Mutex
	upgraded   []string
	events     *[]string
	eventsMu   *sync.Mutex
	stdout     bytes.Buffer
	ioCallback func(id string) string
}

func (m *parallelTestManager) Name() string                         { return m.name }
func (m *parallelTestManager) Aliases() []string                    { return nil }
func (m *parallelTestManager) IsAvailable(ctx context.Context) bool { return true }
func (m *parallelTestManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *parallelTestManager) ValidateID(id string) error { return nil }
func (m *parallelTestManager) ShouldDefer(id string) bool {
	return m.deferIDs != nil && m.deferIDs[id]
}
func (m *parallelTestManager) UpgradePackage(ctx context.Context, id string) error {
	cur := m.activeNow.Add(1)
	for {
		prev := m.maxActive.Load()
		if cur <= prev || m.maxActive.CompareAndSwap(prev, cur) {
			break
		}
	}
	if m.eventsMu != nil && m.events != nil {
		m.eventsMu.Lock()
		*m.events = append(*m.events, fmt.Sprintf("start:%s:%s", m.name, id))
		m.eventsMu.Unlock()
	}

	time.Sleep(m.delay)

	m.mu.Lock()
	m.upgraded = append(m.upgraded, id)
	m.mu.Unlock()

	if m.eventsMu != nil && m.events != nil {
		m.eventsMu.Lock()
		*m.events = append(*m.events, fmt.Sprintf("end:%s:%s", m.name, id))
		m.eventsMu.Unlock()
	}
	m.activeNow.Add(-1)
	return nil
}

func TestExecutor_ParallelismAndDeferral(t *testing.T) {
	var activeNow, maxActive atomic.Int32
	var events []string
	var eventsMu sync.Mutex

	m1 := &parallelTestManager{
		name: "mgr1", delay: 30 * time.Millisecond,
		activeNow: &activeNow, maxActive: &maxActive,
		events: &events, eventsMu: &eventsMu,
	}
	m2 := &parallelTestManager{
		name: "mgr2", delay: 30 * time.Millisecond,
		activeNow: &activeNow, maxActive: &maxActive,
		events: &events, eventsMu: &eventsMu,
		deferIDs: map[string]bool{"m2-shell": true},
	}
	m3 := &parallelTestManager{
		name: "mgr3", delay: 30 * time.Millisecond,
		activeNow: &activeNow, maxActive: &maxActive,
		events: &events, eventsMu: &eventsMu,
	}

	reg := pm.NewRegistry(m1, m2, m3)

	targets := []pm.PackageUpdate{
		{Id: "m1-a", ManagerName: "mgr1"},
		{Id: "m2-shell", ManagerName: "mgr2"}, // deferred -> must run in Phase 2
		{Id: "m1-b", ManagerName: "mgr1"},     // sequential within mgr1
		{Id: "m2-a", ManagerName: "mgr2"},
		{Id: "m3-a", ManagerName: "mgr3"},
	}

	// 1. Concurrency = 0 (parallel across all 3 non-deferred managers)
	var stdout, stderr bytes.Buffer
	execAll := NewExecutor(reg, &stdout, &stderr)
	execAll.Concurrency = 0
	summary := execAll.Execute(context.Background(), &Plan{Targets: targets})

	if summary.Succeeded != 5 || summary.Total != 5 {
		t.Fatalf("expected 5 succeeded, got %+v", summary)
	}
	if maxActive.Load() < 2 {
		t.Errorf("expected parallel execution across managers (maxActive >= 2), got %d", maxActive.Load())
	}
	// Verify Results preserve exact plan.Targets order
	for i, tgt := range targets {
		if summary.Results[i].Package.Id != tgt.Id {
			t.Errorf("result[%d] = %q, want %q (plan.Targets order)", i, summary.Results[i].Package.Id, tgt.Id)
		}
	}
	// Verify deferred m2-shell started only after all Phase 1 events ended
	shellStartIdx := -1
	lastNonDeferredEndIdx := -1
	for idx, ev := range events {
		if ev == "start:mgr2:m2-shell" {
			shellStartIdx = idx
		} else if strings.HasPrefix(ev, "end:") && !strings.HasSuffix(ev, "m2-shell") {
			lastNonDeferredEndIdx = idx
		}
	}
	if shellStartIdx == -1 || shellStartIdx < lastNonDeferredEndIdx {
		t.Errorf("expected deferred m2-shell to start after all non-deferred upgrades, events=%v", events)
	}
	// Verify within mgr1, m1-a completed before m1-b started
	m1aEnd, m1bStart := -1, -1
	for idx, ev := range events {
		if ev == "end:mgr1:m1-a" {
			m1aEnd = idx
		}
		if ev == "start:mgr1:m1-b" {
			m1bStart = idx
		}
	}
	if m1aEnd == -1 || m1bStart == -1 || m1bStart < m1aEnd {
		t.Errorf("expected sequential execution within mgr1, events=%v", events)
	}

	// 2. Concurrency = 1 (serial)
	activeNow.Store(0)
	maxActive.Store(0)
	events = nil
	execSerial := NewExecutor(reg, &stdout, &stderr)
	execSerial.Concurrency = 1
	summarySerial := execSerial.Execute(context.Background(), &Plan{Targets: targets})
	if summarySerial.Succeeded != 5 {
		t.Fatalf("serial execution expected 5 succeeded, got %+v", summarySerial)
	}
	if maxActive.Load() != 1 {
		t.Errorf("expected maxActive == 1 for Concurrency=1, got %d", maxActive.Load())
	}

	// 3. Concurrency = 2 (bounded)
	activeNow.Store(0)
	maxActive.Store(0)
	events = nil
	execBounded := NewExecutor(reg, &stdout, &stderr)
	execBounded.Concurrency = 2
	summaryBounded := execBounded.Execute(context.Background(), &Plan{Targets: targets})
	if summaryBounded.Succeeded != 5 {
		t.Fatalf("bounded execution expected 5 succeeded, got %+v", summaryBounded)
	}
	if maxActive.Load() > 2 {
		t.Errorf("expected maxActive <= 2 for Concurrency=2, got %d", maxActive.Load())
	}
}
