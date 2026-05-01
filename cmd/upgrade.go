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
	Run: func(cmd *cobra.Command, args []string) {
		if upgradeAll {
			fmt.Println("Upgrading all available packages...")
			err := winget.UpgradeAll()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading packages: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Successfully upgraded all packages.")
			return
		}

		if len(args) > 0 {
			packageId := args[0]
			fmt.Printf("Upgrading package: %s...\n", packageId)
			err := winget.UpgradePackage(packageId)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading package %s: %v\n", packageId, err)
				os.Exit(1)
			}
			fmt.Printf("Successfully upgraded package: %s\n", packageId)
			return
		}

		// Interactive selection mode
		fmt.Println("Fetching available updates...")
		updates, err := winget.GetAvailableUpdates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching updates: %v\n", err)
			os.Exit(1)
		}

		if len(updates) == 0 {
			fmt.Println("No updates available.")
			return
		}

		// Prepare survey options
		var options []string
		// Map option strings back to package IDs
		optionToID := make(map[string]string)

		// Calculate padding for tabular alignment
		var maxNameLen, maxIdLen, maxVersionLen int
		for _, pkg := range updates {
			if len(pkg.Name) > maxNameLen { maxNameLen = len(pkg.Name) }
			if len(pkg.Id) > maxIdLen { maxIdLen = len(pkg.Id) }
			if len(pkg.Version) > maxVersionLen { maxVersionLen = len(pkg.Version) }
		}
		
		// Ensure minimum widths
		if maxNameLen < 10 { maxNameLen = 10 }
		if maxIdLen < 10 { maxIdLen = 10 }
		if maxVersionLen < 5 { maxVersionLen = 5 }

		// Dynamic format string for perfectly aligned columns
		formatStr := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds -> %%s", maxNameLen, maxIdLen, maxVersionLen)

		for _, pkg := range updates {
			label := fmt.Sprintf(formatStr, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion)
			options = append(options, label)
			optionToID[label] = pkg.Id
		}

		exitOption := "❌ Cancel (Do not upgrade any packages)"
		options = append(options, exitOption)

		var selectedOptions []string
		prompt := &survey.MultiSelect{
			Message: "Select packages to upgrade (Space to select, Enter to submit, submit empty to cancel):",
			Options: options,
		}
		err = survey.AskOne(prompt, &selectedOptions)
		if err != nil {
			fmt.Println("Selection canceled or failed.")
			return
		}

		if len(selectedOptions) == 0 {
			fmt.Println("No packages selected. Canceled.")
			return
		}

		// Check if the user selected the cancel option
		for _, opt := range selectedOptions {
			if opt == exitOption {
				fmt.Println("Cancel option selected. Aborting upgrades.")
				return
			}
		}

		fmt.Printf("Upgrading %d packages...\n", len(selectedOptions))
		for _, opt := range selectedOptions {
			id := optionToID[opt]
			fmt.Printf("\n--- Upgrading: %s ---\n", id)
			err := winget.UpgradePackage(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", id, err)
			} else {
				fmt.Printf("Successfully upgraded %s\n", id)
			}
		}
		
		fmt.Println("\nAll selected upgrades have completed.")
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().BoolVarP(&upgradeAll, "all", "a", false, "Upgrade all available packages")
}
