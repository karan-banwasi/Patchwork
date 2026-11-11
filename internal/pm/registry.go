package pm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/version"
)

// DefaultFetchTimeout is the default maximum duration allowed for a single package manager to check for updates.
const DefaultFetchTimeout = 2 * time.Minute

// Registry manages the set of supported package managers.
type Registry struct {
	managers         []Manager
	disabledManagers map[string]bool
	FetchTimeout     time.Duration
	SkipRefresh      bool
	Offline          bool
}

// NewRegistry creates a new Registry populated with the provided managers.
func NewRegistry(managers ...Manager) *Registry {
	return &Registry{
		managers:         managers,
		disabledManagers: make(map[string]bool),
	}
}

// SetDisabled configures the list of package managers to disable.
func (r *Registry) SetDisabled(names []string) {
	if r == nil {
		return
	}
	if r.disabledManagers == nil {
		r.disabledManagers = make(map[string]bool)
	} else {
		for k := range r.disabledManagers {
			delete(r.disabledManagers, k)
		}
	}
	for _, n := range names {
		trimmed := strings.ToLower(strings.TrimSpace(n))
		if trimmed != "" {
			r.disabledManagers[trimmed] = true
		}
	}
}

// IsDisabled reports whether a package manager is disabled in configuration.
func (r *Registry) IsDisabled(name string) bool {
	if r == nil || len(r.disabledManagers) == 0 {
		return false
	}
	norm := strings.ToLower(strings.TrimSpace(name))
	if norm == "" {
		return false
	}
	if r.disabledManagers[norm] {
		return true
	}
	for _, m := range r.managers {
		if strings.EqualFold(m.Name(), norm) {
			if r.disabledManagers[strings.ToLower(m.Name())] {
				return true
			}
			for _, a := range m.Aliases() {
				if r.disabledManagers[strings.ToLower(a)] {
					return true
				}
			}
		}
		for _, a := range m.Aliases() {
			if strings.EqualFold(a, norm) {
				if r.disabledManagers[strings.ToLower(m.Name())] {
					return true
				}
				if r.disabledManagers[strings.ToLower(a)] {
					return true
				}
			}
		}
	}
	return false
}

// Register adds a package manager to the registry.
func (r *Registry) Register(m Manager) {
	r.managers = append(r.managers, m)
}

// All returns all registered package managers in registration order.
func (r *Registry) All() []Manager {
	out := make([]Manager, len(r.managers))
	copy(out, r.managers)
	return out
}

// ConfigureOptions applies runtime upgrade options to all registered managers
// implementing OptionsConfigurable.
func (r *Registry) ConfigureOptions(opts UpgradeOptions) {
	if r == nil {
		return
	}
	for _, m := range r.managers {
		if oc, ok := m.(OptionsConfigurable); ok {
			oc.ConfigureOptions(opts)
		}
	}
}

// SetOffline configures offline mode on the registry and any registered managers implementing OfflineConfigurable.
func (r *Registry) SetOffline(offline bool) {
	if r == nil {
		return
	}
	r.Offline = offline
	if offline {
		r.SkipRefresh = true
	}
	for _, m := range r.managers {
		if oc, ok := m.(OfflineConfigurable); ok {
			oc.SetOffline(offline)
		}
	}
}

// GetManager returns a registered package manager by its canonical name or any of its aliases (case-insensitive).
func (r *Registry) GetManager(name string) (Manager, bool) {
	norm := strings.ToLower(strings.TrimSpace(name))
	if norm == "" {
		return nil, false
	}
	for _, m := range r.managers {
		if strings.EqualFold(m.Name(), norm) {
			return m, true
		}
		for _, alias := range m.Aliases() {
			if strings.EqualFold(alias, norm) {
				return m, true
			}
		}
	}
	return nil, false
}

// ManagerNames returns the canonical names of all registered package managers.
func (r *Registry) ManagerNames() []string {
	names := make([]string, len(r.managers))
	for i, m := range r.managers {
		names[i] = m.Name()
	}
	return names
}

// ShouldDefer reports whether upgrading pkg should be deferred to the end of an upgrade queue
// by delegating to the package manager's Deferrer implementation if available.
func (r *Registry) ShouldDefer(pkg PackageUpdate) bool {
	if r == nil {
		return false
	}
	mgr, ok := r.GetManager(pkg.ManagerName)
	if !ok {
		return false
	}
	if d, ok := mgr.(Deferrer); ok {
		return d.ShouldDefer(pkg.Id)
	}
	return false
}

// ActiveManagers returns all registered package managers that are available on the system.
// Availability checks run concurrently while preserving registration order.
func (r *Registry) ActiveManagers(ctx context.Context) []Manager {
	if len(r.managers) == 0 {
		return nil
	}
	availFlags := make([]bool, len(r.managers))
	var wg sync.WaitGroup
	for i, m := range r.managers {
		wg.Add(1)
		go func(idx int, mgr Manager) {
			defer wg.Done()
			availFlags[idx] = mgr.IsAvailable(ctx)
		}(i, m)
	}
	wg.Wait()

	var active []Manager
	for i, m := range r.managers {
		if availFlags[i] && !r.IsDisabled(m.Name()) {
			active = append(active, m)
		}
	}
	return active
}

// ManagerTimeout resolves the effective timeout for a manager:
// 1. If r != nil and r.FetchTimeout > 0, returns r.FetchTimeout (explicit user/CLI override)
// 2. If mgr implements TimeoutProvider and returns > 0, returns tp.FetchTimeout()
// 3. Otherwise returns DefaultFetchTimeout (2m)
func (r *Registry) ManagerTimeout(mgr Manager) time.Duration {
	if r != nil && r.FetchTimeout > 0 {
		return r.FetchTimeout
	}
	if tp, ok := mgr.(TimeoutProvider); ok {
		if d := tp.FetchTimeout(); d > 0 {
			return d
		}
	}
	return DefaultFetchTimeout
}

func (r *Registry) managerTimeout(mgr Manager) time.Duration {
	return r.ManagerTimeout(mgr)
}

// FetchUpdates queries package managers for available updates, optionally filtering by manager names.
// If filter is empty, all active managers on the system are queried concurrently.
// If filter is provided, only specified managers are queried; unknown or unavailable managers return an error.
// Returns collected updates, all aggregated warnings (refresh failures, partial lookups, non-fatal manager errors),
// and a fatal error only if all targeted managers fail.
func (r *Registry) FetchUpdates(ctx context.Context, filter ...string) ([]PackageUpdate, []error, error) {
	var targetManagers []Manager

	var cleanFilter []string
	for _, f := range filter {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			cleanFilter = append(cleanFilter, trimmed)
		}
	}

	if len(cleanFilter) == 0 {
		targetManagers = r.ActiveManagers(ctx)
		if len(targetManagers) == 0 {
			return nil, nil, nil
		}
	} else {
		seen := make(map[string]bool)
		for _, name := range cleanFilter {
			mgr, exists := r.GetManager(name)
			if !exists {
				return nil, nil, fmt.Errorf("unknown package manager %q (supported: %s)", name, strings.Join(r.ManagerNames(), ", "))
			}
			if r.IsDisabled(mgr.Name()) {
				return nil, nil, fmt.Errorf("package manager %q is disabled in configuration", name)
			}
			if !mgr.IsAvailable(ctx) {
				return nil, nil, fmt.Errorf("package manager %q is not installed or available on this system", name)
			}
			if !seen[mgr.Name()] {
				seen[mgr.Name()] = true
				targetManagers = append(targetManagers, mgr)
			}
		}
	}

	type fetchResult struct {
		updates  []PackageUpdate
		warnings []error
		err      error
	}

	ch := make(chan fetchResult, len(targetManagers))

	for _, m := range targetManagers {
		if r.Offline {
			if nd, ok := m.(NetworkDependent); ok && nd.RequiresNetwork() {
				ch <- fetchResult{
					updates:  nil,
					warnings: []error{fmt.Errorf("[%s] offline mode: skipping remote package update checks", m.Name())},
					err:      nil,
				}
				continue
			}
		}

		go func(mgr Manager) {
			timeout := r.ManagerTimeout(mgr)
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			var refreshWarning error
			if !r.SkipRefresh {
				if refresher, ok := mgr.(Refresher); ok {
					if refErr := refresher.Refresh(cctx); refErr != nil {
						if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
							refreshWarning = fmt.Errorf("[%s] refresh timed out after %s: %w", mgr.Name(), timeout, refErr)
						} else {
							refreshWarning = fmt.Errorf("[%s] refresh failed: %w", mgr.Name(), refErr)
						}
					}
				}
			}

			var (
				updates       []PackageUpdate
				fetchWarnings []error
				fetchErr      error
			)

			if wf, ok := mgr.(WarningFetcher); ok {
				updates, fetchWarnings, fetchErr = wf.GetAvailableUpdatesWithWarnings(cctx)
			} else {
				updates, fetchErr = mgr.GetAvailableUpdates(cctx)
			}

			if fetchErr != nil {
				if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
					fetchErr = fmt.Errorf("timed out after %s", timeout)
				}
				fetchErr = fmt.Errorf("[%s] %w", mgr.Name(), fetchErr)
			}

			var combinedWarnings []error
			if refreshWarning != nil {
				combinedWarnings = append(combinedWarnings, refreshWarning)
			}
			if len(fetchWarnings) > 0 {
				combinedWarnings = append(combinedWarnings, fetchWarnings...)
			}

			ch <- fetchResult{
				updates:  updates,
				warnings: combinedWarnings,
				err:      fetchErr,
			}
		}(m)
	}

	var (
		allUpdates  []PackageUpdate
		allWarnings []error
		managerErrs []error
		seen        = make(map[string]bool)
	)

	for i := 0; i < len(targetManagers); i++ {
		res := <-ch
		if len(res.warnings) > 0 {
			allWarnings = append(allWarnings, res.warnings...)
		}
		if res.err != nil {
			managerErrs = append(managerErrs, res.err)
		} else {
			for _, u := range res.updates {
				if u.Version != "" && u.AvailableVersion != "" && !version.IsUpgrade(u.ManagerName, u.Version, u.AvailableVersion) {
					continue
				}
				key := u.ManagerName + ":" + u.Id
				if !seen[key] {
					seen[key] = true
					allUpdates = append(allUpdates, u)
				}
			}
		}
	}

	// Deterministically sort updates by ManagerName, then by package Name
	sort.SliceStable(allUpdates, func(i, j int) bool {
		if allUpdates[i].ManagerName != allUpdates[j].ManagerName {
			return allUpdates[i].ManagerName < allUpdates[j].ManagerName
		}
		return strings.ToLower(allUpdates[i].Name) < strings.ToLower(allUpdates[j].Name)
	})

	if len(managerErrs) > 0 && len(managerErrs) == len(targetManagers) {
		return nil, allWarnings, fmt.Errorf("failed to fetch updates: %w", errors.Join(managerErrs...))
	}

	if len(managerErrs) > 0 {
		allWarnings = append(allWarnings, managerErrs...)
	}

	return allUpdates, allWarnings, nil
}

// FetchAllUpdatesWithWarnings queries all active package managers concurrently and returns
// collected updates, any per-manager partial failure warnings, and a fatal error if all active managers failed.
func (r *Registry) FetchAllUpdatesWithWarnings(ctx context.Context) ([]PackageUpdate, []error, error) {
	return r.FetchUpdates(ctx)
}

// FetchAllUpdates queries all active package managers concurrently for available updates.
func (r *Registry) FetchAllUpdates(ctx context.Context) ([]PackageUpdate, error) {
	updates, _, err := r.FetchUpdates(ctx)
	return updates, err
}
