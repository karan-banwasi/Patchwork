package cmd

import (
	"context"
	"io"
	"os"
	"os/signal"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var version = "dev" // overridden by -ldflags

var registryOverride *pm.Registry

// getDefaultRegistry initializes the default registry of supported package managers for the current OS.
func getDefaultRegistry() *pm.Registry {
	if registryOverride != nil {
		return registryOverride
	}
	reg := pm.NewRegistry(defaultManagers()...)
	if cfg, err := config.Load(); err == nil && cfg != nil && len(cfg.DisabledManagers) > 0 {
		reg.SetDisabled(cfg.DisabledManagers)
	}
	return reg
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
		Long:         `Patchwork iterates through packages that have available updates and helps manage them across 24 supported package managers: Homebrew, Mac App Store (mas), Winget, Scoop, Chocolatey, PowerShell PSResource, APT, DNF, Pacman, Zypper, Flatpak, Snap, npm Global, pnpm Global, Bun Global, .NET Global Tools, uv Python Tools, pipx Python Apps, Rust Cargo Crates, Rustup Toolchains, RubyGems, mise, asdf, and VS Code Extensions. Automatically detects host platform and queries only relevant managers.`,
		SilenceUsage: true,
	}

	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(
		newCheckCmd(),
		newUpgradeCmd(),
		newCleanupCmd(),
		newDoctorCmd(),
		newConfigCmd(),
		newPinCmd(),
		newUnpinCmd(),
		newPinnedCmd(),
		newCompletionCmd(),
		newVersionCmd(),
		newSelfUpdateCmd(),
	)

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cmd := NewRootCmd()
	err := cmd.ExecuteContext(ctx)
	if err != nil || (ctx != nil && ctx.Err() != nil) {
		os.Exit(exitcodes.Determine(ctx, err))
	}
}
