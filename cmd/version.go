package cmd

import (
	"runtime"

	"github.com/spf13/cobra"
)

// newVersionCmd returns the version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Patchwork",
		Long:  `Print full version and build information for Patchwork.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("pw version %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		},
	}
}
