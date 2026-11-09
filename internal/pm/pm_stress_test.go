package pm_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type stressManager struct {
	name         string
	aliases      []string
	available    bool
	timeout      time.Duration
	refreshDelay time.Duration
	refreshErr   error
	fetchDelay   time.Duration
	fetchErr     error
	updates      []pm.PackageUpdate
	warnings     []error
	callCount    int64
}

func (m *stressManager) Name() string      { return m.name }
func (m *stressManager) Aliases() []string { return m.aliases }
func (m *stressManager) IsAvailable(ctx context.Context) bool {
	return m.available
}

func (m *stressManager) FetchTimeout() time.Duration {
	return m.timeout
}

func (m *stressManager) Refresh(ctx context.Context) error {
	if m.refreshDelay > 0 {
		select {
		case <-time.After(m.refreshDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.refreshErr
}

func (m *stressManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	atomic.AddInt64(&m.callCount, 1)
	if m.fetchDelay > 0 {
		select {
		case <-time.After(m.fetchDelay):
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	return m.updates, m.warnings, m.fetchErr
}

func (m *stressManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	u, _, err := m.GetAvailableUpdatesWithWarnings(ctx)
	return u, err
}

func (m *stressManager) UpgradePackage(ctx context.Context, id string) error {
	return nil
}

func (m *stressManager) ValidateID(id string) error {
	return nil
}

// 1. STRESS TEST: Concurrency Safety & High Load
func TestStress_FetchUpdates_Concurrency(t *testing.T) {
	const numManagers = 50
	managers := make([]pm.Manager, numManagers)
	for i := 0; i < numManagers; i++ {
		mgrName := fmt.Sprintf("mgr-%02d", i)
		var updates []pm.PackageUpdate
		var warnings []error
		var fetchErr error

		switch i % 5 {
		case 0:
			// Normal manager with updates
			updates = []pm.PackageUpdate{
				{Name: fmt.Sprintf("pkg-%d", i), Id: fmt.Sprintf("pkg.%d", i), ManagerName: mgrName},
				// Duplicate ID within manager to test deduplication under concurrency
				{Name: fmt.Sprintf("pkg-%d", i), Id: fmt.Sprintf("pkg.%d", i), ManagerName: mgrName},
			}
		case 1:
			// Manager with warnings
			warnings = []error{fmt.Errorf("warning from %s", mgrName)}
			updates = []pm.PackageUpdate{
				{Name: fmt.Sprintf("pkg-%d", i), Id: fmt.Sprintf("pkg.%d", i), ManagerName: mgrName},
			}
		case 2:
			// Manager with fetch error
			fetchErr = fmt.Errorf("network failure in %s", mgrName)
		case 3:
			// Manager with refresh delay
			updates = []pm.PackageUpdate{
				{Name: fmt.Sprintf("pkg-%d", i), Id: fmt.Sprintf("pkg.%d", i), ManagerName: mgrName},
			}
		case 4:
			// Shared duplicate package name across managers
			updates = []pm.PackageUpdate{
				{Name: "shared-pkg", Id: "shared.id", ManagerName: mgrName},
			}
		}

		managers[i] = &stressManager{
			name:       mgrName,
			available:  true,
			timeout:    500 * time.Millisecond,
			updates:    updates,
			warnings:   warnings,
			fetchErr:   fetchErr,
			fetchDelay: time.Duration(i%5) * 5 * time.Millisecond,
		}
	}

	reg := pm.NewRegistry(managers...)

	// Run FetchUpdates repeatedly and concurrently from multiple callers
	var outerWg sync.WaitGroup
	const concurrentCallers = 10
	for c := 0; c < concurrentCallers; c++ {
		outerWg.Add(1)
		go func(callerID int) {
			defer outerWg.Done()
			for iter := 0; iter < 5; iter++ {
				updates, warnings, err := reg.FetchUpdates(context.Background())
				if err != nil {
					t.Errorf("caller %d iter %d: expected success, got error: %v", callerID, iter, err)
					return
				}
				if len(updates) == 0 {
					t.Errorf("caller %d iter %d: expected updates, got 0", callerID, iter)
				}
				if len(warnings) == 0 {
					t.Errorf("caller %d iter %d: expected warnings, got 0", callerID, iter)
				}
			}
		}(c)
	}
	outerWg.Wait()
}

// 2. STRESS TEST: TimeoutProvider Precedence
func TestStress_TimeoutProvider_Precedence(t *testing.T) {
	mgr1 := &stressManager{
		name:      "mgr-short",
		available: true,
		timeout:   100 * time.Millisecond,
	}
	mgr2 := &stressManager{
		name:      "mgr-zero",
		available: true,
		timeout:   0, // zero timeout -> default 2m
	}
	mgr3 := &stressManager{
		name:      "mgr-neg",
		available: true,
		timeout:   -5 * time.Second, // negative -> default 2m
	}

	reg := pm.NewRegistry(mgr1, mgr2, mgr3)

	// Case 1: reg.FetchTimeout == 0 (CLI default)
	if d := reg.ManagerTimeout(mgr1); d != 100*time.Millisecond {
		t.Errorf("mgr1 expected 100ms, got %v", d)
	}
	if d := reg.ManagerTimeout(mgr2); d != pm.DefaultFetchTimeout {
		t.Errorf("mgr2 expected DefaultFetchTimeout, got %v", d)
	}
	if d := reg.ManagerTimeout(mgr3); d != pm.DefaultFetchTimeout {
		t.Errorf("mgr3 expected DefaultFetchTimeout, got %v", d)
	}

	// Case 2: reg.FetchTimeout > 0 (CLI flag override)
	reg.FetchTimeout = 15 * time.Second
	if d := reg.ManagerTimeout(mgr1); d != 15*time.Second {
		t.Errorf("mgr1 expected 15s override, got %v", d)
	}
	if d := reg.ManagerTimeout(mgr2); d != 15*time.Second {
		t.Errorf("mgr2 expected 15s override, got %v", d)
	}
	if d := reg.ManagerTimeout(mgr3); d != 15*time.Second {
		t.Errorf("mgr3 expected 15s override, got %v", d)
	}

	// Case 3: reg.FetchTimeout < 0 (invalid flag / negative)
	reg.FetchTimeout = -10 * time.Second
	if d := reg.ManagerTimeout(mgr1); d != 100*time.Millisecond {
		t.Errorf("mgr1 expected 100ms when FetchTimeout < 0, got %v", d)
	}
}

// 3. STRESS TEST: Context Cancellation & Exceeded Deadlines
func TestStress_FetchUpdates_ContextCancellation(t *testing.T) {
	slowMgr := &stressManager{
		name:       "slow",
		available:  true,
		fetchDelay: 5 * time.Second,
	}
	fastMgr := &stressManager{
		name:      "fast",
		available: true,
		updates: []pm.PackageUpdate{
			{Name: "quick", Id: "quick", ManagerName: "fast"},
		},
	}

	reg := pm.NewRegistry(slowMgr, fastMgr)
	reg.FetchTimeout = 50 * time.Millisecond

	// Even with slow manager, FetchUpdates returns fast updates and timeout warning
	start := time.Now()
	updates, warnings, err := reg.FetchUpdates(context.Background())
	dur := time.Since(start)

	if err != nil {
		t.Fatalf("expected partial success, got err: %v", err)
	}
	if dur > 1*time.Second {
		t.Errorf("expected quick timeout under 1s, took %v", dur)
	}
	if len(updates) != 1 || updates[0].Id != "quick" {
		t.Errorf("expected fast update, got %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "timed out after") {
		t.Errorf("expected timeout warning, got %v", warnings)
	}
}

// 4. STRESS TEST: Fatal Error When ALL Managers Fail
func TestStress_FetchUpdates_AllFailFatal(t *testing.T) {
	fail1 := &stressManager{name: "f1", available: true, fetchErr: errors.New("f1 died")}
	fail2 := &stressManager{name: "f2", available: true, fetchErr: errors.New("f2 died")}

	reg := pm.NewRegistry(fail1, fail2)
	updates, warnings, err := reg.FetchUpdates(context.Background())

	if err == nil {
		t.Fatalf("expected fatal error when all managers fail, got nil")
	}
	if updates != nil {
		t.Errorf("expected nil updates, got %v", updates)
	}
	if !strings.Contains(err.Error(), "failed to fetch updates") {
		t.Errorf("unexpected error format: %v", err)
	}
	_ = warnings
}

// 5. STRESS TEST: Empty Managers List
func TestStress_FetchUpdates_EmptyRegistry(t *testing.T) {
	reg := pm.NewRegistry()
	updates, warnings, err := reg.FetchUpdates(context.Background())
	if err != nil {
		t.Errorf("expected nil error on empty registry, got %v", err)
	}
	if updates != nil || warnings != nil {
		t.Errorf("expected nil updates and warnings on empty registry")
	}
}
