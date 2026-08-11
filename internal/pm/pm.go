package pm

import (
	"context"
)

// PackageUpdate represents an available update for a package managed by a package manager.
type PackageUpdate struct {
	Name             string
	Id               string
	Version          string
	AvailableVersion string
	Source           string
	ManagerName      string
}

// Manager defines the common interface that all package managers (Winget, Scoop, Chocolatey, etc.) must implement.
type Manager interface {
	// Name returns the display name of the package manager (e.g. "winget", "scoop").
	Name() string

	// IsAvailable checks whether the package manager CLI executable is installed and available on the system.
	IsAvailable(ctx context.Context) bool

	// GetAvailableUpdates queries the package manager for packages that have updates available.
	GetAvailableUpdates(ctx context.Context) ([]PackageUpdate, error)

	// UpgradePackage upgrades a specific package by its ID.
	UpgradePackage(ctx context.Context, id string) error

	// UpgradeAll upgrades all packages managed by this package manager.
	UpgradeAll(ctx context.Context) error
}
