package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/schedule"
)

func setupScheduleTestHooks(t *testing.T) {
	t.Helper()
	origRunner := scheduleRunnerFactory
	origGOOS := scheduleGOOS
	origEnable := scheduleEnableFn
	origDisable := scheduleDisableFn
	origReg := scheduleRegisteredFn
	origNotify := scheduleNotifyFn

	t.Cleanup(func() {
		scheduleRunnerFactory = origRunner
		scheduleGOOS = origGOOS
		scheduleEnableFn = origEnable
		scheduleDisableFn = origDisable
		scheduleRegisteredFn = origReg
		scheduleNotifyFn = origNotify
	})
}

func TestScheduleCmd_EnableAndDisable(t *testing.T) {
	setupScheduleTestHooks(t)
	tmpDir := t.TempDir()
	t.Setenv("PATCHWORK_SCHEDULE_STATE", filepath.Join(tmpDir, "schedule.json"))

	var enabledDur time.Duration
	var enabledNotify bool
	registered := false

	scheduleGOOS = func() string { return "linux" }
	scheduleEnableFn = func(ctx context.Context, runner pm.Runner, goos string, interval time.Duration, notify bool) error {
		enabledDur = interval
		enabledNotify = notify
		registered = true
		return nil
	}
	scheduleDisableFn = func(ctx context.Context, runner pm.Runner, goos string) error {
		registered = false
		return nil
	}
	scheduleRegisteredFn = func(ctx context.Context, runner pm.Runner, goos string) (bool, error) {
		return registered, nil
	}

	// Invalid interval rejected
	cmdBad := NewRootCmd()
	cmdBad.SetArgs([]string{"schedule", "enable", "--interval", "5m"})
	if err := cmdBad.Execute(); err == nil {
		t.Fatalf("expected error for interval < 15m")
	}

	// Enable with daily & notify=false
	var stdout bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"schedule", "enable", "--interval", "daily", "--notify=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schedule enable failed: %v", err)
	}
	if enabledDur != 24*time.Hour || enabledNotify {
		t.Fatalf("unexpected enable hook args: dur=%s notify=%v", enabledDur, enabledNotify)
	}
	if !strings.Contains(stdout.String(), "interval: daily") {
		t.Errorf("unexpected enable output: %s", stdout.String())
	}

	st, err := schedule.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if !st.Enabled || st.Interval != "daily" || st.Notify {
		t.Fatalf("unexpected saved state: %+v", st)
	}

	// Disable
	stdout.Reset()
	cmdDis := NewRootCmd()
	cmdDis.SetOut(&stdout)
	cmdDis.SetArgs([]string{"schedule", "disable"})
	if err := cmdDis.Execute(); err != nil {
		t.Fatalf("schedule disable failed: %v", err)
	}
	if registered {
		t.Fatalf("expected registered=false after disable")
	}
	st, err = schedule.LoadState()
	if err != nil {
		t.Fatalf("LoadState after disable failed: %v", err)
	}
	if st.Enabled || st.Interval != "daily" {
		t.Fatalf("expected Enabled=false with interval preserved, got %+v", st)
	}
}

func TestScheduleCmd_StatusJSONAndBar(t *testing.T) {
	setupScheduleTestHooks(t)
	tmpDir := t.TempDir()
	t.Setenv("PATCHWORK_SCHEDULE_STATE", filepath.Join(tmpDir, "schedule.json"))
	t.Setenv("PATCHWORK_CACHE", filepath.Join(tmpDir, "updates-cache.json"))
	t.Setenv("PATCHWORK_CONFIG", filepath.Join(tmpDir, "config.json"))

	scheduleRegisteredFn = func(ctx context.Context, runner pm.Runner, goos string) (bool, error) {
		return true, nil
	}

	// Before any cache or run -> --bar prints "pw: not checked"
	var out bytes.Buffer
	cmdInit := NewRootCmd()
	cmdInit.SetOut(&out)
	cmdInit.SetArgs([]string{"schedule", "status", "--bar"})
	if err := cmdInit.Execute(); err != nil {
		t.Fatalf("status --bar failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "pw: not checked" {
		t.Errorf("expected 'pw: not checked', got %q", out.String())
	}

	// Save state and 3 cached updates
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	st := &schedule.State{
		Enabled:   true,
		Interval:  "6h",
		Notify:    true,
		LastRun:   now,
		LastCount: 3,
		UpdatedAt: now,
	}
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}
	updates := []pm.PackageUpdate{
		{Id: "pkg1", Name: "Pkg1", Version: "1.0", AvailableVersion: "2.0", ManagerName: "winget"},
		{Id: "pkg2", Name: "Pkg2", Version: "1.0", AvailableVersion: "1.1", ManagerName: "scoop"},
		{Id: "pkg3", Name: "Pkg3", Version: "2.0", AvailableVersion: "3.0", ManagerName: "npm"},
	}
	if err := config.SaveUpdateCache(updates); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	// --bar with 3 updates
	out.Reset()
	cmdBar3 := NewRootCmd()
	cmdBar3.SetOut(&out)
	cmdBar3.SetArgs([]string{"schedule", "status", "--bar"})
	if err := cmdBar3.Execute(); err != nil {
		t.Fatalf("status --bar failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "pw: 3 updates" {
		t.Errorf("expected 'pw: 3 updates', got %q", out.String())
	}

	// --json
	out.Reset()
	cmdJSON := NewRootCmd()
	cmdJSON.SetOut(&out)
	cmdJSON.SetArgs([]string{"schedule", "status", "--json"})
	if err := cmdJSON.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
	var parsed scheduleStatusJSON
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal status json: %v (raw: %s)", err, out.String())
	}
	if parsed.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version 1.0.0, got %q", parsed.SchemaVersion)
	}
	if !parsed.Enabled || !parsed.Registered || parsed.Interval != "6h" || !parsed.Notify || parsed.CachedCount != 3 || !parsed.CacheValid {
		t.Errorf("unexpected status JSON payload: %+v", parsed)
	}

	// --bar with 1 update
	if err := config.SaveUpdateCache(updates[:1]); err != nil {
		t.Fatalf("save 1-item cache: %v", err)
	}
	out.Reset()
	cmdBar1 := NewRootCmd()
	cmdBar1.SetOut(&out)
	cmdBar1.SetArgs([]string{"schedule", "status", "--bar"})
	if err := cmdBar1.Execute(); err != nil {
		t.Fatalf("status --bar (1) failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "pw: 1 update" {
		t.Errorf("expected 'pw: 1 update', got %q", out.String())
	}

	// Human-readable status output
	out.Reset()
	cmdHuman := NewRootCmd()
	cmdHuman.SetOut(&out)
	cmdHuman.SetArgs([]string{"schedule", "status"})
	if err := cmdHuman.Execute(); err != nil {
		t.Fatalf("status human failed: %v", err)
	}
	for _, want := range []string{"Enabled:", "OS Registered:", "Interval:", "Notifications:", "Last Run:", "Last Count:", "Cached Updates:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in status output:\n%s", want, out.String())
		}
	}

	// --bar with 0 updates
	if err := config.SaveUpdateCache(nil); err != nil {
		t.Fatalf("save empty cache: %v", err)
	}
	out.Reset()
	cmdBar0 := NewRootCmd()
	cmdBar0.SetOut(&out)
	cmdBar0.SetArgs([]string{"schedule", "status", "--bar"})
	if err := cmdBar0.Execute(); err != nil {
		t.Fatalf("status --bar failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "pw: up to date" {
		t.Errorf("expected 'pw: up to date', got %q", out.String())
	}

	// --json and --bar mutually exclusive
	cmdBoth := NewRootCmd()
	cmdBoth.SetArgs([]string{"schedule", "status", "--json", "--bar"})
	if err := cmdBoth.Execute(); err == nil {
		t.Errorf("expected error when --json and --bar are combined")
	}
}

func TestScheduleCmd_RunUpdatesCacheStateAndNotifies(t *testing.T) {
	setupScheduleTestHooks(t)
	tmpDir := t.TempDir()
	t.Setenv("PATCHWORK_SCHEDULE_STATE", filepath.Join(tmpDir, "schedule.json"))
	t.Setenv("PATCHWORK_CACHE", filepath.Join(tmpDir, "updates-cache.json"))
	t.Setenv("PATCHWORK_CONFIG", filepath.Join(tmpDir, "config.json"))

	mockMgr := &mockCheckManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0.0", AvailableVersion: "1.1.0", ManagerName: "mockpm"},
			{Id: "pkg.two", Name: "PkgTwo", Version: "2.0.0", AvailableVersion: "2.1.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var notifiedTitle, notifiedMsg string
	notifyCalls := 0
	scheduleNotifyFn = func(ctx context.Context, runner pm.Runner, goos, title, message string) error {
		notifyCalls++
		notifiedTitle = title
		notifiedMsg = message
		return nil
	}

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"schedule", "run", "--quiet"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("schedule run failed: %v", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("expected no stdout in --quiet mode, got %q", stdout.String())
	}
	if notifyCalls != 1 {
		t.Fatalf("expected 1 notification call, got %d", notifyCalls)
	}
	if notifiedTitle == "" || !strings.Contains(notifiedMsg, "2 updates available") {
		t.Errorf("unexpected notification: title=%q msg=%q", notifiedTitle, notifiedMsg)
	}

	// Verify cache updated
	cached, ok := config.LoadUpdateCache(time.Hour)
	if !ok || len(cached) != 2 {
		t.Fatalf("expected 2 cached updates, got ok=%v len=%d", ok, len(cached))
	}

	// Verify state updated
	st, err := schedule.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if st.LastCount != 2 || st.LastRun.IsZero() {
		t.Fatalf("expected LastCount=2 and non-zero LastRun, got %+v", st)
	}

	// Run again with --notify=false -> should NOT notify
	notifyCalls = 0
	cmdNoNotify := NewRootCmd()
	cmdNoNotify.SetOut(&stdout)
	cmdNoNotify.SetArgs([]string{"schedule", "run", "--notify=false"})
	if err := cmdNoNotify.Execute(); err != nil {
		t.Fatalf("schedule run --notify=false failed: %v", err)
	}
	if notifyCalls != 0 {
		t.Errorf("expected 0 notification calls when --notify=false, got %d", notifyCalls)
	}
	if !strings.Contains(stdout.String(), "Found 2 available update(s).") {
		t.Errorf("expected non-quiet summary output, got %q", stdout.String())
	}

	// Notification failure should warn without failing run
	scheduleNotifyFn = func(ctx context.Context, runner pm.Runner, goos, title, message string) error {
		return errors.New("dbus unavailable")
	}
	stderr.Reset()
	cmdWarn := NewRootCmd()
	cmdWarn.SetOut(&bytes.Buffer{})
	cmdWarn.SetErr(&stderr)
	cmdWarn.SetArgs([]string{"schedule", "run", "--notify=true"})
	if err := cmdWarn.Execute(); err != nil {
		t.Fatalf("expected notification failure to warn non-fatally, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to send desktop notification") {
		t.Errorf("expected notification warning on stderr, got: %s", stderr.String())
	}

	// Zero updates -> should NOT notify
	mockMgr.updates = nil
	notifyCalls = 0
	scheduleNotifyFn = func(ctx context.Context, runner pm.Runner, goos, title, message string) error {
		notifyCalls++
		notifiedTitle = title
		notifiedMsg = message
		return nil
	}
	cmdZero := NewRootCmd()
	cmdZero.SetOut(&bytes.Buffer{})
	cmdZero.SetArgs([]string{"schedule", "run", "--notify=true"})
	if err := cmdZero.Execute(); err != nil {
		t.Fatalf("schedule run with 0 updates failed: %v", err)
	}
	if notifyCalls != 0 {
		t.Errorf("expected 0 notifications when 0 updates available, got %d", notifyCalls)
	}

	// Single update notification formatting + honoring persisted Notify=false + --offline/--timeout
	mockMgr.updates = []pm.PackageUpdate{
		{Id: "pkg.one", Name: "PkgOne", Version: "1.0.0", AvailableVersion: "1.1.0", ManagerName: "mockpm"},
	}
	notifyCalls = 0
	cmdOne := NewRootCmd()
	cmdOne.SetOut(&bytes.Buffer{})
	cmdOne.SetArgs([]string{"schedule", "run", "--offline", "--timeout", "15s"})
	if err := cmdOne.Execute(); err != nil {
		t.Fatalf("schedule run single update failed: %v", err)
	}
	if notifyCalls != 1 || !strings.Contains(notifiedMsg, "1 update available: PkgOne (1.0.0 -> 1.1.0 via mockpm)") {
		t.Errorf("unexpected single-update notification: calls=%d msg=%q", notifyCalls, notifiedMsg)
	}

	// Persisted state with Notify=false is honored when --notify flag is omitted
	st2, _ := schedule.LoadState()
	st2.Notify = false
	if err := st2.Save(); err != nil {
		t.Fatalf("save state Notify=false: %v", err)
	}
	notifyCalls = 0
	cmdStateNotifyOff := NewRootCmd()
	cmdStateNotifyOff.SetOut(&bytes.Buffer{})
	cmdStateNotifyOff.SetArgs([]string{"schedule", "run"})
	if err := cmdStateNotifyOff.Execute(); err != nil {
		t.Fatalf("schedule run with state Notify=false failed: %v", err)
	}
	if notifyCalls != 0 {
		t.Errorf("expected 0 notifications when persisted state Notify=false, got %d", notifyCalls)
	}
}
