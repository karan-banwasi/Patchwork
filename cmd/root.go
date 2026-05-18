package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev" // overridden by -ldflags

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "pw",
	Version: version,
	Short:   "Patchwork is a CLI tool to manage package updates",
	Long:    `Patchwork iterates through packages that have available updates and helps manage them across different package managers like winget.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
}
