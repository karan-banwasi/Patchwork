package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var (
	checkManager        string
	byManager           bool
	checkJSON           bool
	checkQuiet          bool
	checkIncludePinned  bool
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for available updates",
	Long:  `Iterate through supported package managers and check for available updates without installing them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		registry := getDefaultRegistry()
		out := cmd.OutOrStdout()
		silentStatus := checkJSON || checkQuiet

		var updates []pm.PackageUpdate
		var err error

		if checkManager != "" {
			mgr, exists := registry.GetManager(checkManager)
			if !exists {
				return fmt.Errorf("unknown package manager %q (supported: %s)", checkManager, strings.Join(registry.ManagerNames(), ", "))
			}
			if !mgr.IsAvailable(cmd.Context()) {
				return fmt.Errorf("package manager %q is not installed or available on this system", checkManager)
			}
			if !silentStatus {
				fmt.Fprintf(out, "Checking for available updates using %s...\n", mgr.Name())
			}
			updates, err = mgr.GetAvailableUpdates(cmd.Context())
		} else {
			if !silentStatus {
				fmt.Fprintln(out, "Checking for available updates across package managers...")
			}
			updates, err = registry.FetchAllUpdates(cmd.Context())
		}

		if err != nil {
			return fmt.Errorf("error checking for updates: %w", err)
		}

		if checkManager == "" {
			_ = config.SaveUpdateCache(updates)
		}

		var pinnedCount int
		if !checkIncludePinned {
			updates, pinnedCount = filterOutPinned(updates)
		}
		if updates == nil {
			updates = []pm.PackageUpdate{}
		}

		if checkJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(updates)
		}

		if checkQuiet {
			for _, pkg := range updates {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", pkg.Id, pkg.Version, pkg.AvailableVersion, pkg.ManagerName)
			}
			return nil
		}

		if len(updates) == 0 {
			if pinnedCount > 0 {
				fmt.Fprintf(out, "All packages are up to date (%d pinned package(s) skipped).\n", pinnedCount)
			} else {
				fmt.Fprintln(out, "All packages are up to date.")
			}
			return nil
		}

		if pinnedCount > 0 {
			fmt.Fprintf(out, "Found %d packages with available updates (%d pinned skipped):\n", len(updates), pinnedCount)
		} else {
			fmt.Fprintf(out, "Found %d packages with available updates:\n", len(updates))
		}

		if byManager {
			grouped := make(map[string][]pm.PackageUpdate)
			var groupOrder []string
			for _, pkg := range updates {
				mgrName := pkg.ManagerName
				if mgrName == "" {
					mgrName = "other"
				}
				if _, exists := grouped[mgrName]; !exists {
					groupOrder = append(groupOrder, mgrName)
				}
				grouped[mgrName] = append(grouped[mgrName], pkg)
			}

			for _, mgrName := range groupOrder {
				pkgs := grouped[mgrName]
				pluralSuffix := "s"
				if len(pkgs) == 1 {
					pluralSuffix = ""
				}
				fmt.Fprintf(out, "\n--- %s (%d package%s) ---\n\n", mgrName, len(pkgs), pluralSuffix)
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

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringVarP(&checkManager, "manager", "m", "", "Target a specific package manager")
	checkCmd.Flags().BoolVar(&byManager, "by-manager", false, "Group updates by package manager")
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "Output available updates as JSON")
	checkCmd.Flags().BoolVarP(&checkQuiet, "quiet", "q", false, "Output only tab-separated package IDs and versions")
	checkCmd.Flags().BoolVar(&checkIncludePinned, "include-pinned", false, "Include pinned packages in the check output")
	_ = checkCmd.RegisterFlagCompletionFunc("manager", completeManagerNames)
}
