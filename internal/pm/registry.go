package pm

import (
	"context"
	"fmt"
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

// FetchAllUpdates queries all active package managers for available updates.
func (r *Registry) FetchAllUpdates(ctx context.Context) ([]PackageUpdate, error) {
	var allUpdates []PackageUpdate
	var errs []error

	for _, m := range r.ActiveManagers(ctx) {
		updates, err := m.GetAvailableUpdates(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] error fetching updates: %w", m.Name(), err))
			continue
		}
		allUpdates = append(allUpdates, updates...)
	}

	if len(errs) > 0 && len(allUpdates) == 0 {
		return nil, fmt.Errorf("failed to fetch updates: %v", errs)
	}

	return allUpdates, nil
}
