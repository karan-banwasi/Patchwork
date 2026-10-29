package pm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
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

// GetManager returns a registered package manager by its name (case-insensitive, supporting aliases).
func (r *Registry) GetManager(name string) (Manager, bool) {
	norm := strings.ToLower(strings.TrimSpace(name))
	switch norm {
	case "chocolatey":
		norm = "choco"
	case "powershell", "psresource", "psresourceget":
		norm = "psresource"
	case "dotnet-tool":
		norm = "dotnet"
	case "uv-tool":
		norm = "uv"
	case "rust":
		norm = "cargo"
	}
	for _, m := range r.managers {
		if strings.EqualFold(m.Name(), norm) {
			return m, true
		}
	}
	return nil, false
}

// ManagerNames returns the names of all registered package managers.
func (r *Registry) ManagerNames() []string {
	names := make([]string, len(r.managers))
	for i, m := range r.managers {
		names[i] = m.Name()
	}
	return names
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

	if len(errs) > 0 {
		// Only fail completely if every active manager encountered an error
		if len(errs) == len(active) {
			return nil, fmt.Errorf("failed to fetch updates: %w", errors.Join(errs...))
		}
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		}
	}

	return allUpdates, nil
}
