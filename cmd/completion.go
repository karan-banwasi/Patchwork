package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func completeManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	reg := getDefaultRegistry()
	return reg.ManagerNames(), cobra.ShellCompDirectiveNoFileComp
}

func completeAvailablePackageIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	reg := getDefaultRegistry()
	updates, err := reg.FetchAllUpdates(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	updates, _ = filterOutPinned(updates)
	var completions []string
	for _, u := range updates {
		if toComplete == "" || strings.HasPrefix(strings.ToLower(u.Id), strings.ToLower(toComplete)) {
			completions = append(completions, fmt.Sprintf("%s\t%s (%s -> %s, %s)", u.Id, u.Name, u.Version, u.AvailableVersion, u.ManagerName))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

var completionCmd = &cobra.Command{
	Use:   "completion [powershell|bash|zsh|fish]",
	Short: "Generate shell completion script",
	Long: `To load completions in PowerShell:

  pw completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output to your PowerShell $PROFILE:

  pw completion powershell >> $PROFILE
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"powershell", "bash", "zsh", "fish"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		switch args[0] {
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(out)
		case "bash":
			return rootCmd.GenBashCompletion(out)
		case "zsh":
			return rootCmd.GenZshCompletion(out)
		case "fish":
			return rootCmd.GenFishCompletion(out, true)
		}
		return fmt.Errorf("unsupported shell %q", args[0])
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(completionCmd)
	_ = os.Stdout // keep os import if needed
}
