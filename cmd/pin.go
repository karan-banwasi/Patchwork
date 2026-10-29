package cmd

import (
	"fmt"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

// filterOutPinned removes packages that are pinned in the user's configuration.
func filterOutPinned(updates []pm.PackageUpdate) ([]pm.PackageUpdate, int) {
	cfg, err := config.Load()
	if err != nil || len(cfg.PinnedPackages) == 0 {
		return updates, 0
	}
	var filtered []pm.PackageUpdate
	var skipped int
	for _, u := range updates {
		if cfg.IsPinned(u.Id, u.Name) {
			skipped++
			continue
		}
		filtered = append(filtered, u)
	}
	return filtered, skipped
}

var pinCmd = &cobra.Command{
	Use:               "pin <package-id>",
	Short:             "Pin a package to ignore it during check and upgrade",
	Long:              `Add a package ID or name to the Patchwork ignore list so it is skipped during 'pw check' and 'pw upgrade'.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAvailablePackageIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if !cfg.Pin(id) {
			cmd.Printf("Package %q is already pinned.\n", id)
			return nil
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		cmd.Printf("Pinned package %q (will be skipped during check and upgrade).\n", id)
		return nil
	},
}

var unpinCmd = &cobra.Command{
	Use:   "unpin <package-id>",
	Short: "Unpin a previously pinned package",
	Long:  `Remove a package ID or name from the Patchwork ignore list so its updates are shown again.`,
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := config.Load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cfg.PinnedPackages, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if !cfg.Unpin(id) {
			return fmt.Errorf("package %q is not currently pinned", id)
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		cmd.Printf("Unpinned package %q.\n", id)
		return nil
	},
}

var pinnedCmd = &cobra.Command{
	Use:   "pinned",
	Short: "List all pinned (ignored) packages",
	Long:  `Display all packages currently pinned in the Patchwork configuration.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if len(cfg.PinnedPackages) == 0 {
			cmd.Println("No packages are currently pinned.")
			return nil
		}
		cmd.Printf("Pinned packages (%d):\n", len(cfg.PinnedPackages))
		for _, p := range cfg.PinnedPackages {
			cmd.Printf("  - %s\n", p)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
	rootCmd.AddCommand(pinnedCmd)
}
