package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/winget"
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

		if upgradeAll {
			fmt.Println("Upgrading all available packages...")
			err := winget.UpgradeAll(cmd.Context())
			if err != nil {
				return fmt.Errorf("error upgrading packages: %w", err)
			}
			fmt.Println("Successfully upgraded all packages.")
			return nil
		}

		if len(args) > 0 {
			packageId := args[0]
			fmt.Printf("Upgrading package: %s...\n", packageId)
			err := winget.UpgradePackage(cmd.Context(), packageId)
			if err != nil {
				return fmt.Errorf("error upgrading package %s: %w", packageId, err)
			}
			fmt.Printf("Successfully upgraded package: %s\n", packageId)
			return nil
		}

		// Interactive selection mode
		fmt.Println("Fetching available updates...")
		updates, err := winget.GetAvailableUpdates(cmd.Context())
		if err != nil {
			return fmt.Errorf("error fetching updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("No updates available.")
			return nil
		}

		// Prepare survey options
		var options []string
		// Map option strings back to package IDs
		optionToID := make(map[string]string)

		maxNameLen, maxIdLen, maxVersionLen := getDefaultPackageColumnWidths(updates)

		for _, pkg := range updates {
			label := formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen)
			options = append(options, label)
			optionToID[label] = pkg.Id
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
			id := optionToID[opt]
			fmt.Printf("\n--- Upgrading: %s ---\n", id)
			err := winget.UpgradePackage(cmd.Context(), id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", id, err)
				hadError = true
			} else {
				fmt.Printf("Successfully upgraded %s\n", id)
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
