package cmd

import (
	"context"
	"io"
	"os"
	"os/signal"

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

// getDefaultRegistry initializes the default registry of supported package managers.
func getDefaultRegistry() *pm.Registry {
	return pm.NewRegistry(
		winget.NewWingetManager(),
		scoop.NewScoopManager(),
		choco.NewChocoManager(),
		npm.NewNpmManager(),
		dotnet.NewDotnetManager(),
		uv.NewUvManager(),
		cargo.NewCargoManager(),
		psresource.NewPSResourceManager(),
	)
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
		Long:         `Patchwork iterates through packages that have available updates and helps manage them across different package managers including Winget, Scoop, and Chocolatey.`,
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
