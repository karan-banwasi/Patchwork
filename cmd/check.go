package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

type checkOptions struct {
	manager       string
	byManager     bool
	json          bool
	quiet         bool
	includePinned bool
	noRefresh     bool
	timeout       time.Duration
}

func newCheckCmd() *cobra.Command {
	var opts checkOptions

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		Long:  `Iterate through supported package managers and check for available updates without installing them.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := getDefaultRegistry()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			configureRegistryIO(registry, cmd.InOrStdin(), out, errOut)

			if opts.timeout > 0 {
				registry.FetchTimeout = opts.timeout
			}
			registry.SkipRefresh = opts.noRefresh

			silentStatus := opts.json || opts.quiet

			var updates []pm.PackageUpdate
			var warnings []error
			var err error

			if opts.manager != "" {
				mgr, exists := registry.GetManager(opts.manager)
				if !exists {
					return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
				}
				if !mgr.IsAvailable(cmd.Context()) {
					return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
				}
				if !silentStatus {
					fmt.Fprintf(out, "Checking for available updates using %s...\n", mgr.Name())
				}
				cctx, cancel := context.WithTimeout(cmd.Context(), registry.FetchTimeout)
				defer cancel()
				if !opts.noRefresh {
					if refresher, ok := mgr.(pm.Refresher); ok {
						_ = refresher.Refresh(cctx)
					}
				}
				updates, err = mgr.GetAvailableUpdates(cctx)
				if err != nil {
					return err
				}
			} else {
				if !silentStatus {
					fmt.Fprintln(out, "Checking for available updates across all supported package managers...")
				}
				updates, warnings, err = registry.FetchAllUpdatesWithWarnings(cmd.Context())
				if err != nil {
					return err
				}
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
				updates, pinnedCount = filterOutPinned(updates)
			}

			if opts.json {
				type jsonPackage struct {
					Name             string `json:"name"`
					ID               string `json:"id"`
					Version          string `json:"version"`
					AvailableVersion string `json:"available_version"`
					Manager          string `json:"manager"`
				}
				type jsonOutput struct {
					Updates []jsonPackage `json:"updates"`
					Count   int           `json:"count"`
					Pinned  int           `json:"pinned_skipped,omitempty"`
				}
				outList := make([]jsonPackage, 0, len(updates))
				for _, u := range updates {
					outList = append(outList, jsonPackage{
						Name:             u.Name,
						ID:               u.Id,
						Version:          u.Version,
						AvailableVersion: u.AvailableVersion,
						Manager:          u.ManagerName,
					})
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(jsonOutput{
					Updates: outList,
					Count:   len(outList),
					Pinned:  pinnedCount,
				})
			}

			if opts.quiet {
				for _, u := range updates {
					fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", u.Id, u.Version, u.AvailableVersion, u.ManagerName)
				}
				return nil
			}

			if len(updates) == 0 {
				if pinnedCount > 0 {
					fmt.Fprintf(out, "No updates available (%d pinned package(s) skipped; use --include-pinned to see them).\n", pinnedCount)
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
				return nil
			}

			fmt.Fprintln(out)
			maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(updates)

			headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds   %%s\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)

			fmt.Fprintf(out, headerFmt, "Name", "ID", "Version", "Available", "Manager")
			fmt.Fprintf(out, headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), strings.Repeat("-", maxAvailLen), "-------")

			for _, pkg := range updates {
				fmt.Fprintf(out, "  %s\n", formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen))
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
	cmd.Flags().DurationVar(&opts.timeout, "timeout", pm.DefaultFetchTimeout, "Per-manager timeout when querying updates")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}
