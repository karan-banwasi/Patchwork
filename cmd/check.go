package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/winget"
	"github.com/spf13/cobra"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for available updates",
	Long:  `Iterate through supported package managers and check for available updates without installing them.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Checking for available updates using winget...")

		updates, err := winget.GetAvailableUpdates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
			os.Exit(1)
		}

		if len(updates) == 0 {
			fmt.Println("All packages are up to date.")
			return
		}

		fmt.Printf("Found %d packages with available updates:\n\n", len(updates))
		
		var maxNameLen, maxIdLen, maxVersionLen int
		for _, pkg := range updates {
			if len(pkg.Name) > maxNameLen { maxNameLen = len(pkg.Name) }
			if len(pkg.Id) > maxIdLen { maxIdLen = len(pkg.Id) }
			if len(pkg.Version) > maxVersionLen { maxVersionLen = len(pkg.Version) }
		}

		if maxNameLen < 4 { maxNameLen = 4 }
		if maxIdLen < 2 { maxIdLen = 2 }
		if maxVersionLen < 7 { maxVersionLen = 7 }

		headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds    %%s\n", maxNameLen, maxIdLen, maxVersionLen)
		rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds -> %%s\n", maxNameLen, maxIdLen, maxVersionLen)
		
		fmt.Printf(headerFmt, "Name", "ID", "Version", "Available")
		fmt.Printf(headerFmt, strings.Repeat("-", maxNameLen), strings.Repeat("-", maxIdLen), strings.Repeat("-", maxVersionLen), "---------")
		
		for _, pkg := range updates {
			fmt.Printf(rowFmt, pkg.Name, pkg.Id, pkg.Version, pkg.AvailableVersion)
		}
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
