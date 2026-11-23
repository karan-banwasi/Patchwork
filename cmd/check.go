package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
	semver "github.com/karan-banwasi/patchwork/internal/version"
	"github.com/spf13/cobra"
)

type checkOptions struct {
	manager       string
	byManager     bool
	json          bool
	quiet         bool
	includePinned bool
	noRefresh     bool
	offline       bool
	timeout       time.Duration
	exclude       []string
	level         string
	changelog     bool
}

func newCheckCmd() *cobra.Command {
	var opts checkOptions

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		Long:  `Iterate through supported package managers and check for available updates without installing them.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, cfg := getDefaultRegistryAndConfig()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			configureRegistryIO(registry, cmd.InOrStdin(), out, errOut)

			hydrateCommonDefaults(cmd, cfg, &opts.timeout, &opts.noRefresh, &opts.level, &opts.exclude)

			if _, err := semver.ParseLevel(opts.level); err != nil {
				return err
			}

			if opts.offline {
				opts.noRefresh = true
				registry.SetOffline(true)
			}

			if opts.timeout > 0 {
				registry.FetchTimeout = opts.timeout
			}
			registry.SkipRefresh = opts.noRefresh

			silentStatus := opts.json || opts.quiet

			var filter []string
			if opts.manager != "" {
				mgr, err := resolveManager(cmd.Context(), registry, opts.manager, true, true)
				if err != nil {
					return err
				}
				filter = append(filter, opts.manager)
				if !silentStatus {
					fmt.Fprintf(out, "Checking for available updates using %s...\n", mgr.Name())
				}
			} else {
				if !silentStatus {
					fmt.Fprintln(out, "Checking for available updates across all supported package managers...")
				}
			}

			updates, warnings, err := registry.FetchUpdates(cmd.Context(), filter...)
			if err != nil {
				return err
			}

			for _, w := range warnings {
				fmt.Fprintf(errOut, "Warning: %v\n", w)
			}

			// Persist retrieved updates to cache for shell completions (only when querying all managers without specific filters)
			if opts.manager == "" {
				_ = config.SaveUpdateCache(updates)
			}

			// Filter pinned packages unless explicitly requested
			pinnedCount := 0
			if !opts.includePinned {
				var err error
				updates, pinnedCount, err = filterOutPinned(updates)
				if err != nil {
					return fmt.Errorf("cannot verify pinned packages: %w (use --include-pinned to bypass)", err)
				}
			}

			updates, policySkipped, err := upgrade.FilterPolicies(updates, opts.exclude, opts.level)
			if err != nil {
				return err
			}

			if opts.json {
				type jsonPackage struct {
					Name             string `json:"name"`
					ID               string `json:"id"`
					Version          string `json:"version"`
					AvailableVersion string `json:"available_version"`
					Manager          string `json:"manager"`
					ChangelogURL     string `json:"changelog_url,omitempty"`
				}
				type jsonOutput struct {
					SchemaVersion string        `json:"schema_version"`
					Updates       []jsonPackage `json:"updates"`
					Count         int           `json:"count"`
					Pinned        int           `json:"pinned_skipped,omitempty"`
					Filtered      int           `json:"policy_skipped,omitempty"`
				}
				outList := make([]jsonPackage, 0, len(updates))
				for _, u := range updates {
					jp := jsonPackage{
						Name:             u.Name,
						ID:               u.Id,
						Version:          u.Version,
						AvailableVersion: u.AvailableVersion,
						Manager:          u.ManagerName,
					}
					if opts.changelog {
						jp.ChangelogURL = upgrade.ChangelogURL(u)
					}
					outList = append(outList, jp)
				}
				return encodeIndentedJSON(out, jsonOutput{
					SchemaVersion: upgrade.CurrentSchemaVersion,
					Updates:       outList,
					Count:         len(outList),
					Pinned:        pinnedCount,
					Filtered:      len(policySkipped),
				})
			}

			if opts.quiet {
				for _, u := range updates {
					fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", u.Id, u.Version, u.AvailableVersion, u.ManagerName)
				}
				return nil
			}

			if len(updates) == 0 {
				if pinnedCount > 0 && len(policySkipped) > 0 {
					fmt.Fprintf(out, "No updates available (%d pinned, %d filtered by policy).\n", pinnedCount, len(policySkipped))
				} else if pinnedCount > 0 {
					fmt.Fprintf(out, "No updates available (%d pinned package(s) skipped; use --include-pinned to see them).\n", pinnedCount)
				} else if len(policySkipped) > 0 {
					fmt.Fprintf(out, "No updates available (%d package(s) filtered by policy).\n", len(policySkipped))
				} else {
					fmt.Fprintln(out, "No updates available. Everything is up to date!")
				}
				return nil
			}

			if opts.byManager {
				grouped := make(map[string][]pm.PackageUpdate)
				for _, pkg := range updates {
					grouped[pkg.ManagerName] = append(grouped[pkg.ManagerName], pkg)
				}

				for _, mgrName := range registry.ManagerNames() {
					pkgs, exists := grouped[mgrName]
					if !exists || len(pkgs) == 0 {
						continue
					}

					fmt.Fprintf(out, "\n[%s]\n", mgrName)
					maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(pkgs)

					headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
					fmt.Fprintf(out, headerFmt, "Name", "ID", "Version", "Available")
					fmt.Fprintf(out, headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), strings.Repeat("-", maxAvailLen))
					for _, pkg := range pkgs {
						rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
						fmt.Fprintf(out, rowFmt, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion)
					}
				}
			} else {
				fmt.Fprintln(out)
				maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(updates)

				headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds   %%s\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)

				fmt.Fprintf(out, headerFmt, "Name", "ID", "Version", "Available", "Manager")
				fmt.Fprintf(out, headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), strings.Repeat("-", maxAvailLen), "-------")

				for _, pkg := range updates {
					fmt.Fprintf(out, "  %s\n", formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen))
				}
			}

			if opts.changelog {
				upgrade.RenderChangelogPreview(out, updates)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Target a specific package manager")
	cmd.Flags().BoolVar(&opts.byManager, "by-manager", false, "Group updates by package manager")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output available updates as JSON")
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Output only tab-separated package IDs and versions")
	cmd.Flags().BoolVar(&opts.includePinned, "include-pinned", false, "Include pinned packages in the check output")
	cmd.Flags().BoolVar(&opts.noRefresh, "no-refresh", false, "Skip bucket/repository refresh before checking updates")
	cmd.Flags().BoolVar(&opts.offline, "offline", false, "Disable network access and check using only local caches")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "Per-manager timeout when querying updates (0 uses manager defaults)")
	cmd.Flags().StringSliceVar(&opts.exclude, "exclude", nil, "Exclude packages matching glob pattern (comma-separated or repeatable)")
	cmd.Flags().StringVar(&opts.level, "level", "", "Maximum semver bump level to include (patch, minor, major)")
	cmd.Flags().BoolVar(&opts.changelog, "changelog", false, "Display changelog/release preview URLs for available updates")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)
	_ = cmd.RegisterFlagCompletionFunc("level", completeLevelNames)

	return cmd
}
