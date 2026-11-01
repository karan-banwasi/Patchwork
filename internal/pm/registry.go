package pm

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultFetchTimeout is the default maximum duration allowed for a single package manager to check for updates.
const DefaultFetchTimeout = 2 * time.Minute

// Registry manages the set of supported package managers.
type Registry struct {
	managers     []Manager
	FetchTimeout time.Duration
	SkipRefresh  bool
	Warnings     []error
}

// NewRegistry creates a new Registry populated with the provided managers.
func NewRegistry(managers ...Manager) *Registry {
	return &Registry{
		managers:     managers,
		FetchTimeout: DefaultFetchTimeout,
	}
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

// ShouldDefer reports whether upgrading pkg should be deferred to the end of an upgrade queue.
func (r *Registry) ShouldDefer(pkg PackageUpdate) bool {
	if r != nil {
		if mgr, ok := r.GetManager(pkg.ManagerName); ok {
			if d, ok := mgr.(Deferrer); ok {
				return d.ShouldDefer(pkg.Id)
			}
		}
	}
	lower := strings.ToLower(strings.TrimSpace(pkg.Id))
	return strings.HasPrefix(lower, "microsoft.powershell") ||
		strings.HasPrefix(lower, "microsoft.windowsterminal") ||
		lower == "pwsh" ||
		lower == "powershell" ||
		lower == "windows-terminal"
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
		if availFlags[i] {
			active = append(active, m)
		}
	}
	return active
}

func (r *Registry) managerTimeout(mgr Manager) time.Duration {
	if r.FetchTimeout > 0 {
		return r.FetchTimeout
	}
	if tp, ok := mgr.(TimeoutProvider); ok {
		if d := tp.FetchTimeout(); d > 0 {
			return d
		}
	}
	return DefaultFetchTimeout
}

// FetchAllUpdates queries all active package managers concurrently for available updates.
// Partial-failure warnings are recorded in r.Warnings and can also be retrieved via FetchAllUpdatesWithWarnings.
func (r *Registry) FetchAllUpdates(ctx context.Context) ([]PackageUpdate, error) {
	updates, warnings, err := r.FetchAllUpdatesWithWarnings(ctx)
	r.Warnings = warnings
	return updates, err
}

// FetchAllUpdatesWithWarnings queries all active package managers concurrently and returns
// collected updates, any per-manager partial failure warnings, and a fatal error if all active managers failed.
func (r *Registry) FetchAllUpdatesWithWarnings(ctx context.Context) ([]PackageUpdate, []error, error) {
	active := r.ActiveManagers(ctx)
	if len(active) == 0 {
		r.Warnings = nil
		return nil, nil, nil
	}

	type fetchResult struct {
		updates []PackageUpdate
		err     error
	}

	ch := make(chan fetchResult, len(active))

	for _, m := range active {
		go func(mgr Manager) {
			timeout := r.managerTimeout(mgr)
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			if !r.SkipRefresh {
				if refresher, ok := mgr.(Refresher); ok {
					_ = refresher.Refresh(cctx)
				}
			}

			updates, err := mgr.GetAvailableUpdates(cctx)
			if err != nil {
				if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
					err = fmt.Errorf("timed out after %s", timeout)
				}
				ch <- fetchResult{err: fmt.Errorf("[%s] %w", mgr.Name(), err)}
				return
			}
			if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				ch <- fetchResult{err: fmt.Errorf("[%s] timed out after %s", mgr.Name(), timeout)}
				return
			}
			ch <- fetchResult{updates: updates}
		}(m)
	}

	var allUpdates []PackageUpdate
	var errs []error
	seen := make(map[string]bool)

	for i := 0; i < len(active); i++ {
		res := <-ch
		if res.err != nil {
			errs = append(errs, res.err)
		} else {
			for _, u := range res.updates {
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

	r.Warnings = errs
	if len(errs) > 0 && len(errs) == len(active) {
		return nil, errs, fmt.Errorf("failed to fetch updates: %w", errors.Join(errs...))
	}

	return allUpdates, errs, nil
}
