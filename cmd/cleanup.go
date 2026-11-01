package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

type cleanupOptions struct {
	manager string
}

func newCleanupCmd() *cobra.Command {
	var opts cleanupOptions

	cmd := &cobra.Command{
		Use:     "cleanup",
		Aliases: []string{"clean"},
		Short:   "Clean up old package versions and download caches",
		Long:    `Remove outdated package versions and prune download caches across supported package managers (Scoop, npm, uv) and clear Patchwork's temporary update cache.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			reg := getDefaultRegistry()

			// Always remove Patchwork's temporary completion cache
			_ = os.Remove(config.CachePath())

			var managers []pm.Manager
			if opts.manager != "" {
				mgr, exists := reg.GetManager(opts.manager)
				if !exists {
					return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(reg.ManagerNames(), ", "))
				}
				managers = []pm.Manager{mgr}
			} else {
				managers = reg.All()
			}

			ranAny := false
			for _, mgr := range managers {
				cleaner, ok := mgr.(pm.Cleaner)
				if !ok || !mgr.IsAvailable(cmd.Context()) {
					continue
				}
				ranAny = true
				if err := cleaner.Cleanup(cmd.Context(), out, errOut); err != nil {
					fmt.Fprintf(errOut, "Warning: cleanup for %s encountered an error: %v\n", mgr.Name(), err)
				}
			}

			if !ranAny {
				if opts.manager != "" {
					fmt.Fprintf(out, "No cache cleanup action required or available for %s.\n", opts.manager)
				} else {
					fmt.Fprintln(out, "Cleared Patchwork update cache.")
				}
			} else {
				fmt.Fprintln(out, "\nCleanup complete.")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Clean up a specific package manager")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}
