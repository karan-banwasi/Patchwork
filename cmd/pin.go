package cmd

import (
	"fmt"
	"strings"

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
		if cfg.IsPinnedForManager(u.ManagerName, u.Id, u.Name) {
			skipped++
			continue
		}
		filtered = append(filtered, u)
	}
	return filtered, skipped
}

func newPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "pin <package-id> [package-id...]",
		Short:             "Pin one or more packages to ignore them during check and upgrade",
		Long:              `Add one or more package IDs (or manager-scoped entries like winget:Git.Git) to the Patchwork ignore list so they are skipped during 'pw check' and 'pw upgrade'.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeAvailablePackageIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			anyChanged := false
			for _, id := range args {
				if !cfg.Pin(id) {
					cmd.Printf("Package %q is already pinned.\n", id)
					continue
				}
				anyChanged = true
				cmd.Printf("Pinned package %q (will be skipped during check and upgrade).\n", id)
			}
			if anyChanged {
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}
			return nil
		},
	}
}

func newUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <package-id> [package-id...]",
		Short: "Unpin one or more previously pinned packages",
		Long:  `Remove one or more package IDs (or manager:id entries) from the Patchwork ignore list so their updates are shown again.`,
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cfg, err := config.Load()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			already := make(map[string]bool, len(args))
			for _, a := range args {
				already[strings.ToLower(a)] = true
			}
			var out []string
			for _, p := range cfg.PinnedPackages {
				if !already[strings.ToLower(p)] {
					out = append(out, p)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			var missing []string
			anyChanged := false
			for _, id := range args {
				if !cfg.Unpin(id) {
					missing = append(missing, id)
					continue
				}
				anyChanged = true
				cmd.Printf("Unpinned package %q.\n", id)
			}
			if anyChanged {
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("package %q is not currently pinned", strings.Join(missing, ", "))
			}
			return nil
		},
	}
}

func newPinnedCmd() *cobra.Command {
	return &cobra.Command{
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
				if idx := strings.IndexByte(p, ':'); idx > 0 && idx < len(p)-1 {
					cmd.Printf("  - %s (manager: %s)\n", p[idx+1:], p[:idx])
				} else {
					cmd.Printf("  - %s (all managers)\n", p)
				}
			}
			return nil
		},
	}
}
