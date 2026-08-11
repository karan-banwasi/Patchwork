package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Patchwork",
	Long:  `Print full version and build information for Patchwork.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pw version %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
