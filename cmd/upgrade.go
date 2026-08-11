package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var upgradeAll bool

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:   "upgrade [package-id]",
	Short: "Upgrade available packages",
	Long:  `Upgrade a specific package by providing its ID, or upgrade all packages using the --all flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if upgradeAll && len(args) > 0 {
			return fmt.Errorf("--all and a package ID are mutually exclusive")
		}

		registry := getDefaultRegistry()

		if upgradeAll {
			fmt.Println("Upgrading all available packages across package managers...")
			var hadErr bool
			for _, m := range registry.ActiveManagers(cmd.Context()) {
				fmt.Printf("\n--- Running upgrade all for %s ---\n", m.Name())
				err := m.UpgradeAll(cmd.Context())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error upgrading packages for %s: %v\n", m.Name(), err)
					hadErr = true
				}
			}
			if hadErr {
				return fmt.Errorf("one or more package manager upgrades failed")
			}
			fmt.Println("\nSuccessfully completed upgrade for all packages.")
			return nil
		}

		if len(args) > 0 {
			packageId := args[0]
			fmt.Printf("Upgrading package: %s...\n", packageId)
			var upgraded bool
			var lastErr error
			for _, m := range registry.ActiveManagers(cmd.Context()) {
				err := m.UpgradePackage(cmd.Context(), packageId)
				if err == nil {
					upgraded = true
					fmt.Printf("Successfully upgraded package: %s\n", packageId)
					break
				}
				lastErr = err
			}
			if !upgraded {
				return fmt.Errorf("failed to upgrade package %s: %v", packageId, lastErr)
			}
			return nil
		}

		// Interactive selection mode
		fmt.Println("Fetching available updates...")
		updates, err := registry.FetchAllUpdates(cmd.Context())
		if err != nil {
			return fmt.Errorf("error fetching updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("No updates available.")
			return nil
		}

		// Prepare survey options
		var options []string
		optionToUpdate := make(map[string]pm.PackageUpdate)

		maxNameLen, maxIdLen, maxVersionLen := getDefaultPackageColumnWidths(updates)

		for _, pkg := range updates {
			label := formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen)
			options = append(options, label)
			optionToUpdate[label] = pkg
		}

		var selectedOptions []string
		prompt := &survey.MultiSelect{
			Message: "Select packages to upgrade (Space to select, Enter to submit, submit empty to cancel):",
			Options: options,
		}
		err = survey.AskOne(prompt, &selectedOptions)
		if err != nil {
			fmt.Println("Selection canceled or failed.")
			return nil
		}

		if len(selectedOptions) == 0 {
			fmt.Println("No packages selected. Canceled.")
			return nil
		}

		fmt.Printf("Upgrading %d packages...\n", len(selectedOptions))
		hadError := false
		for _, opt := range selectedOptions {
			pkg := optionToUpdate[opt]
			fmt.Printf("\n--- Upgrading: %s ---\n", pkg.Id)
			mgr, exists := registry.GetManager(pkg.ManagerName)
			if !exists {
				fmt.Fprintf(os.Stderr, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
				hadError = true
				continue
			}
			err := mgr.UpgradePackage(cmd.Context(), pkg.Id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", pkg.Id, err)
				hadError = true
			} else {
				fmt.Printf("Successfully upgraded %s\n", pkg.Id)
			}
		}
		
		fmt.Println("\nAll selected upgrades have completed.")
		if hadError {
			return fmt.Errorf("one or more package upgrades failed")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().BoolVarP(&upgradeAll, "all", "a", false, "Upgrade all available packages")
}
