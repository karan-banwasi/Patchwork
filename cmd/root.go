package cmd

import (
	"context"
	"io"
	"os"
	"os/signal"
	"runtime"

	"github.com/karan-banwasi/patchwork/internal/brew"
	"github.com/karan-banwasi/patchwork/internal/cargo"
	"github.com/karan-banwasi/patchwork/internal/choco"
	"github.com/karan-banwasi/patchwork/internal/dotnet"
	"github.com/karan-banwasi/patchwork/internal/npm"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/psresource"
	"github.com/karan-banwasi/patchwork/internal/scoop"
	"github.com/karan-banwasi/patchwork/internal/uv"
	"github.com/karan-banwasi/patchwork/internal/winget"
	"github.com/spf13/cobra"
)

var version = "dev" // overridden by -ldflags

// getDefaultRegistry initializes the default registry of supported package managers for the current OS.
func getDefaultRegistry() *pm.Registry {
	return getDefaultRegistryForOS(runtime.GOOS)
}

// getDefaultRegistryForOS initializes the registry with managers appropriate for the given OS.
func getDefaultRegistryForOS(goos string) *pm.Registry {
	var managers []pm.Manager

	switch goos {
	case "windows":
		managers = append(managers,
			winget.NewWingetManager(),
			scoop.NewScoopManager(),
			choco.NewChocoManager(),
			psresource.NewPSResourceManager(),
		)
	case "darwin", "linux":
		managers = append(managers,
			brew.NewBrewManager(),
		)
	default:
		managers = append(managers,
			brew.NewBrewManager(),
		)
	}

	// Cross-platform runtime and tool package managers
	managers = append(managers,
		npm.NewNpmManager(),
		dotnet.NewDotnetManager(),
		uv.NewUvManager(),
		cargo.NewCargoManager(),
	)

	return pm.NewRegistry(managers...)
}

// configureRegistryIO attaches the command's standard streams to all managers in the registry.
func configureRegistryIO(reg *pm.Registry, stdin io.Reader, stdout, stderr io.Writer) {
	for _, m := range reg.All() {
		if cfg, ok := m.(pm.IOConfigurable); ok {
			cfg.SetIO(stdin, stdout, stderr)
		}
	}
}

// NewRootCmd returns a newly instantiated root command tree with all subcommands registered.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "pw",
		Version:      version,
		Short:        "Patchwork is a CLI tool to manage package updates",
		Long:         `Patchwork iterates through packages that have available updates and helps manage them across different package managers including Winget, Scoop, Chocolatey, and Homebrew.`,
		SilenceUsage: true,
	}

	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(
		newCheckCmd(),
		newUpgradeCmd(),
		newCleanupCmd(),
		newDoctorCmd(),
		newPinCmd(),
		newUnpinCmd(),
		newPinnedCmd(),
		newCompletionCmd(),
		newVersionCmd(),
		newSelfUpdateCmd(),
	)

	return cmd
}

// rootCmd is provided for backward-compatibility with tests or packages referencing rootCmd directly.
var rootCmd = NewRootCmd()

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cmd := NewRootCmd()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}
