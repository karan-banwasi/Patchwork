package cmd

import (
	"fmt"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/winget"
	"github.com/spf13/cobra"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for available updates",
	Long:  `Iterate through supported package managers and check for available updates without installing them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Checking for available updates using winget...")

		updates, err := winget.GetAvailableUpdates(cmd.Context())
		if err != nil {
			return fmt.Errorf("error checking for updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("All packages are up to date.")
			return nil
		}

		fmt.Printf("Found %d packages with available updates:\n\n", len(updates))
		
		maxNameLen, maxIdLen, maxVersionLen := getDefaultPackageColumnWidths(updates)

		headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%s\n", maxNameLen, maxIdLen, maxVersionLen)
		
		fmt.Printf(headerFmt, "Name", "ID", "Version", "Available")
		fmt.Printf(headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), "---------")
		
		for _, pkg := range updates {
			fmt.Printf("  %s\n", formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
