package pm

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// PackageUpdate represents an available update for a package managed by a package manager.
type PackageUpdate struct {
	Name             string `json:"name"`
	Id               string `json:"id"`
	Version          string `json:"version"`
	AvailableVersion string `json:"available_version"`
	Source           string `json:"source"`
	ManagerName      string `json:"manager"`
}

// Manager defines the common interface that all package managers (Winget, Scoop, Chocolatey, etc.) must implement.
type Manager interface {
	// Name returns the canonical display name of the package manager (e.g. "winget", "scoop").
	Name() string

	// Aliases returns alternative names accepted on the CLI for this package manager.
	Aliases() []string

	// IsAvailable checks whether the package manager CLI executable is installed and available on the system.
	IsAvailable(ctx context.Context) bool

	// GetAvailableUpdates queries the package manager for packages that have updates available.
	GetAvailableUpdates(ctx context.Context) ([]PackageUpdate, error)

	// UpgradePackage upgrades a specific package by its ID.
	UpgradePackage(ctx context.Context, id string) error

	// ValidateID returns an error if id is not a syntactically valid package identifier for this manager.
	ValidateID(id string) error
}

// Locator is an optional interface implemented by managers that can report their binary path and version.
type Locator interface {
	Path() (string, error)
	Version(ctx context.Context) (string, error)
}

// Cleaner is an optional interface implemented by managers that support cache or old-version cleanup.
type Cleaner interface {
	Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error
}

// Deferrer is an optional interface implemented by managers that need to defer self-updating
// host shells or terminals to the end of an upgrade queue.
type Deferrer interface {
	ShouldDefer(id string) bool
}

// Refresher is an optional interface implemented by managers that separate state/bucket refresh from querying.
type Refresher interface {
	Refresh(ctx context.Context) error
}

// BulkUpgrader is an optional interface implemented by managers that can upgrade multiple packages in one invocation.
type BulkUpgrader interface {
	UpgradePackages(ctx context.Context, ids []string) error
}

// IOConfigurable is an optional interface implemented by managers that accept custom I/O streams.
type IOConfigurable interface {
	SetIO(stdin io.Reader, stdout, stderr io.Writer)
}

// TimeoutProvider is an optional interface implemented by managers that customize their fetch timeout.
type TimeoutProvider interface {
	FetchTimeout() time.Duration
}

// Runner abstracts command execution so managers can be tested without invoking external binaries.
type Runner interface {
	Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// ExecRunner is the default Runner backed by os/exec.
type ExecRunner struct{}

// Run executes an external command with the given streams.
func (ExecRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
