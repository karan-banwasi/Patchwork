package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var version = "dev" // overridden by -ldflags

var registryOverride *pm.Registry

// getDefaultRegistryAndConfig loads user config once and returns the active package manager registry and loaded config.
func getDefaultRegistryAndConfig() (*pm.Registry, *config.Config) {
	cfg, _ := config.Load()
	if registryOverride != nil {
		return registryOverride, cfg
	}
	reg := pm.NewRegistry(defaultManagers()...)
	if cfg != nil && len(cfg.DisabledManagers) > 0 {
		reg.SetDisabled(cfg.DisabledManagers)
	}
	return reg, cfg
}

// getDefaultRegistry initializes the default registry of supported package managers for the current OS.
func getDefaultRegistry() *pm.Registry {
	reg, _ := getDefaultRegistryAndConfig()
	return reg
}

// resolveManager looks up a package manager by name or alias and optionally validates enabled/availability state.
func resolveManager(ctx context.Context, reg *pm.Registry, name string, checkDisabled, checkAvailable bool) (pm.Manager, error) {
	mgr, exists := reg.GetManager(name)
	if !exists {
		return nil, fmt.Errorf("unknown package manager %q (supported: %s)", name, strings.Join(reg.ManagerNames(), ", "))
	}
	if checkDisabled && reg.IsDisabled(mgr.Name()) {
		return nil, fmt.Errorf("package manager %q is disabled in configuration", name)
	}
	if checkAvailable && !mgr.IsAvailable(ctx) {
		return nil, fmt.Errorf("package manager %q is not installed or available on this system", name)
	}
	return mgr, nil
}

// hydrateCommonDefaults populates unset command flags from config.Defaults.
func hydrateCommonDefaults(cmd *cobra.Command, cfg *config.Config, timeout *time.Duration, noRefresh *bool, level *string, exclude *[]string) {
	if cfg == nil {
		return
	}
	if timeout != nil && !cmd.Flags().Changed("timeout") && cfg.Defaults.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Defaults.Timeout); err == nil {
			*timeout = d
		}
	}
	if noRefresh != nil && !cmd.Flags().Changed("no-refresh") && cfg.Defaults.NoRefresh != nil {
		*noRefresh = *cfg.Defaults.NoRefresh
	}
	if level != nil && !cmd.Flags().Changed("level") && cfg.Defaults.Level != "" {
		*level = cfg.Defaults.Level
	}
	if exclude != nil && !cmd.Flags().Changed("exclude") && len(cfg.Defaults.Exclude) > 0 {
		*exclude = append([]string(nil), cfg.Defaults.Exclude...)
	}
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
	var noticeHandle *selfUpdateNoticeHandle

	cmd := &cobra.Command{
		Use:          "pw",
		Version:      version,
		Short:        "Patchwork is a CLI tool to manage package updates",
		Long:         `Patchwork iterates through packages that have available updates and helps manage them across 24 supported package managers: Homebrew, Mac App Store (mas), Winget, Scoop, Chocolatey, PowerShell PSResource, APT, DNF, Pacman, Zypper, Flatpak, Snap, npm Global, pnpm Global, Bun Global, .NET Global Tools, uv Python Tools, pipx Python Apps, Rust Cargo Crates, Rustup Toolchains, RubyGems, mise, asdf, and VS Code Extensions. Automatically detects host platform and queries only relevant managers.`,
		SilenceUsage: true,
		PersistentPreRun: func(c *cobra.Command, args []string) {
			noticeHandle = startPassiveSelfUpdateCheck(c)
		},
		PersistentPostRun: func(c *cobra.Command, args []string) {
			finishPassiveSelfUpdateCheck(c, noticeHandle)
		},
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
		newHistoryCmd(),
		newRollbackCmd(),
		newExportCmd(),
		newImportCmd(),
		newCompletionCmd(),
		newVersionCmd(),
		newSelfUpdateCmd(),
		newScheduleCmd(),
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
