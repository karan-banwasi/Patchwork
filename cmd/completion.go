package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/spf13/cobra"
)

func completeManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	reg := getDefaultRegistry()
	return reg.ManagerNames(), cobra.ShellCompDirectiveNoFileComp
}

// completeAvailablePackageIDs reads strictly from the local update cache (populated by `pw check`)
// so that Tab completion never triggers network requests or mutates package manager state.
func completeAvailablePackageIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	alreadySelected := make(map[string]bool, len(args))
	for _, a := range args {
		alreadySelected[strings.ToLower(a)] = true
	}

	updates, ok := config.LoadUpdateCache(24 * time.Hour)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	updates, _ = filterOutPinned(updates)
	var completions []string
	for _, u := range updates {
		if alreadySelected[strings.ToLower(u.Id)] {
			continue
		}
		if toComplete == "" || strings.HasPrefix(strings.ToLower(u.Id), strings.ToLower(toComplete)) {
			completions = append(completions, fmt.Sprintf("%s\t%s (%s -> %s, %s)", u.Id, u.Name, u.Version, u.AvailableVersion, u.ManagerName))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [powershell|bash|zsh|fish]",
		Short: "Generate shell completion script",
		Long: `To load completions in PowerShell:

  pw completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output to your PowerShell $PROFILE:

  pw completion powershell >> $PROFILE

Note: Package ID completions read from the local cache updated by 'pw check'.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"powershell", "bash", "zsh", "fish"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			root := cmd.Root()
			switch args[0] {
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			}
			return fmt.Errorf("unsupported shell %q", args[0])
		},
	}
}
