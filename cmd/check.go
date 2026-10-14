package cmd

import (
	"fmt"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var (
	checkManager string
	byManager    bool
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for available updates",
	Long:  `Iterate through supported package managers and check for available updates without installing them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		registry := getDefaultRegistry()

		var updates []pm.PackageUpdate
		var err error

		if checkManager != "" {
			mgr, exists := registry.GetManager(checkManager)
			if !exists {
				return fmt.Errorf("unknown package manager %q (supported: winget, scoop, choco)", checkManager)
			}
			if !mgr.IsAvailable(cmd.Context()) {
				return fmt.Errorf("package manager %q is not installed or available on this system", checkManager)
			}
			fmt.Printf("Checking for available updates using %s...\n", mgr.Name())
			updates, err = mgr.GetAvailableUpdates(cmd.Context())
		} else {
			fmt.Println("Checking for available updates across package managers...")
			updates, err = registry.FetchAllUpdates(cmd.Context())
		}

		if err != nil {
			return fmt.Errorf("error checking for updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("All packages are up to date.")
			return nil
		}

		fmt.Printf("Found %d packages with available updates:\n", len(updates))

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
				fmt.Printf("\n--- %s (%d package%s) ---\n\n", mgrName, len(pkgs), pluralSuffix)
				maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(pkgs)
				headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
				fmt.Printf(headerFmt, "Name", "ID", "Version", "Available")
				fmt.Printf(headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), strings.Repeat("-", maxAvailLen))
				for _, pkg := range pkgs {
					rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
					fmt.Printf(rowFmt, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion)
				}
			}
			return nil
		}

		fmt.Println()
		maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(updates)

		headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%-%ds   %%s\n", maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)

		fmt.Printf(headerFmt, "Name", "ID", "Version", "Available", "Manager")
		fmt.Printf(headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), strings.Repeat("-", maxAvailLen), "-------")

		for _, pkg := range updates {
			fmt.Printf("  %s\n", formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringVarP(&checkManager, "manager", "m", "", "Target a specific package manager (winget, scoop, choco)")
	checkCmd.Flags().BoolVar(&byManager, "by-manager", false, "Group updates by package manager")
}
