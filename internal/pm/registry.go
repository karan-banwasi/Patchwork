package pm

import (
	"context"
	"fmt"
	"os"
)

// Registry manages the set of supported package managers.
type Registry struct {
	managers []Manager
}

// NewRegistry creates a new Registry populated with the provided managers.
func NewRegistry(managers ...Manager) *Registry {
	return &Registry{
		managers: managers,
	}
}

// Register adds a package manager to the registry.
func (r *Registry) Register(m Manager) {
	r.managers = append(r.managers, m)
}

// GetManager returns a registered package manager by its name.
func (r *Registry) GetManager(name string) (Manager, bool) {
	for _, m := range r.managers {
		if m.Name() == name {
			return m, true
		}
	}
	return nil, false
}

// ActiveManagers returns all registered package managers that are available on the system.
func (r *Registry) ActiveManagers(ctx context.Context) []Manager {
	var active []Manager
	for _, m := range r.managers {
		if m.IsAvailable(ctx) {
			active = append(active, m)
		}
	}
	return active
}

// FetchAllUpdates queries all active package managers concurrently for available updates.
func (r *Registry) FetchAllUpdates(ctx context.Context) ([]PackageUpdate, error) {
	active := r.ActiveManagers(ctx)
	if len(active) == 0 {
		return nil, nil
	}

	type fetchResult struct {
		updates []PackageUpdate
		err     error
	}

	ch := make(chan fetchResult, len(active))

	for _, m := range active {
		go func(mgr Manager) {
			updates, err := mgr.GetAvailableUpdates(ctx)
			if err != nil {
				ch <- fetchResult{err: fmt.Errorf("[%s] %w", mgr.Name(), err)}
				return
			}
			ch <- fetchResult{updates: updates}
		}(m)
	}

	var allUpdates []PackageUpdate
	var errs []error

	for i := 0; i < len(active); i++ {
		res := <-ch
		if res.err != nil {
			errs = append(errs, res.err)
		} else {
			allUpdates = append(allUpdates, res.updates...)
		}
	}

	if len(errs) > 0 {
		if len(allUpdates) == 0 {
			return nil, fmt.Errorf("failed to fetch updates: %v", errs)
		}
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		}
	}

	return allUpdates, nil
}
