package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/spf13/cobra"
)

var cleanupManager string

func runCleanupForManager(ctxCmd *cobra.Command, name string) (bool, error) {
	out := ctxCmd.OutOrStdout()
	switch name {
	case "scoop":
		exe, _ := resolveManagerBinaryAndVersion("scoop")
		if exe == "" {
			return false, nil
		}
		fmt.Fprintln(out, "--- Cleaning up Scoop (old versions & download cache) ---")
		cmd1 := exec.CommandContext(ctxCmd.Context(), exe, "cleanup", "*")
		cmd1.Stdout = out
		cmd1.Stderr = os.Stderr
		_ = cmd1.Run()

		cmd2 := exec.CommandContext(ctxCmd.Context(), exe, "cache", "rm", "*")
		cmd2.Stdout = out
		cmd2.Stderr = os.Stderr
		if err := cmd2.Run(); err != nil {
			return true, err
		}
		return true, nil

	case "npm":
		exe, _ := resolveManagerBinaryAndVersion("npm")
		if exe == "" {
			return false, nil
		}
		fmt.Fprintln(out, "--- Cleaning up npm cache ---")
		cmd := exec.CommandContext(ctxCmd.Context(), exe, "cache", "clean", "--force")
		cmd.Stdout = out
		cmd.Stderr = os.Stderr
		return true, cmd.Run()

	case "uv":
		exe, _ := resolveManagerBinaryAndVersion("uv")
		if exe == "" {
			return false, nil
		}
		fmt.Fprintln(out, "--- Pruning uv cache ---")
		cmd := exec.CommandContext(ctxCmd.Context(), exe, "cache", "prune")
		cmd.Stdout = out
		cmd.Stderr = os.Stderr
		return true, cmd.Run()
	}
	return false, nil
}

var cleanupCmd = &cobra.Command{
	Use:     "cleanup",
	Aliases: []string{"clean"},
	Short:   "Clean up old package versions and download caches",
	Long:    `Remove outdated package versions and prune download caches across supported package managers (Scoop, npm, uv) and clear Patchwork's temporary update cache.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		reg := getDefaultRegistry()

		// Always remove Patchwork's temporary completion cache
		_ = os.Remove(config.CachePath())

		targets := []string{"scoop", "npm", "uv"}
		if cleanupManager != "" {
			mgr, exists := reg.GetManager(cleanupManager)
			if !exists {
				return fmt.Errorf("unknown package manager %q (supported: %s)", cleanupManager, strings.Join(reg.ManagerNames(), ", "))
			}
			targets = []string{mgr.Name()}
		}

		ranAny := false
		for _, name := range targets {
			ran, err := runCleanupForManager(cmd, name)
			if ran {
				ranAny = true
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: cleanup for %s encountered an error: %v\n", name, err)
			}
		}

		if !ranAny {
			if cleanupManager != "" {
				fmt.Fprintf(out, "No cache cleanup action required or available for %s.\n", cleanupManager)
			} else {
				fmt.Fprintln(out, "Cleared Patchwork update cache.")
			}
		} else {
			fmt.Fprintln(out, "\nCleanup complete.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.Flags().StringVarP(&cleanupManager, "manager", "m", "", "Clean up a specific package manager")
	_ = cleanupCmd.RegisterFlagCompletionFunc("manager", completeManagerNames)
}
