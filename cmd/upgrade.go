package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var (
	upgradeAll       bool
	upgradeManager   string
	upgradeByManager bool
)

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:   "upgrade [package-id]",
	Short: "Upgrade available packages",
	Long:  `Upgrade a specific package by providing its ID, or upgrade all packages using the --all flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if upgradeAll && len(args) > 0 {
			return fmt.Errorf("--all and a package ID are mutually exclusive")
		}
		if upgradeByManager && len(args) > 0 {
			return fmt.Errorf("--by-manager and a package ID are mutually exclusive")
		}

		registry := getDefaultRegistry()

		var targetedMgr pm.Manager
		if upgradeManager != "" {
			mgr, exists := registry.GetManager(upgradeManager)
			if !exists {
				return fmt.Errorf("unknown package manager %q (supported: winget, scoop, choco)", upgradeManager)
			}
			if !mgr.IsAvailable(cmd.Context()) {
				return fmt.Errorf("package manager %q is not installed or available on this system", upgradeManager)
			}
			targetedMgr = mgr
		}

		if upgradeAll {
			if targetedMgr != nil {
				fmt.Printf("Upgrading all available packages for %s...\n", targetedMgr.Name())
				err := targetedMgr.UpgradeAll(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to upgrade packages for %s: %w", targetedMgr.Name(), err)
				}
				fmt.Printf("\nSuccessfully completed upgrade for all %s packages.\n", targetedMgr.Name())
				return nil
			}

			fmt.Println("Upgrading all available packages across package managers...")
			var hadErr bool
			for _, m := range registry.ActiveManagers(cmd.Context()) {
				fmt.Printf("\n--- Running upgrade all for %s ---\n", m.Name())
				err := m.UpgradeAll(cmd.Context())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error upgrading packages for %s: %v\n", m.Name(), err)
					hadErr = true
				}
			}
			if hadErr {
				return fmt.Errorf("one or more package manager upgrades failed")
			}
			fmt.Println("\nSuccessfully completed upgrade for all packages.")
			return nil
		}

		if len(args) > 0 {
			packageId := args[0]
			if targetedMgr != nil {
				fmt.Printf("Upgrading package: %s using %s...\n", packageId, targetedMgr.Name())
				err := targetedMgr.UpgradePackage(cmd.Context(), packageId)
				if err != nil {
					return fmt.Errorf("failed to upgrade package %s with %s: %w", packageId, targetedMgr.Name(), err)
				}
				fmt.Printf("Successfully upgraded package: %s\n", packageId)
				return nil
			}

			fmt.Printf("Upgrading package: %s...\n", packageId)

			// Try to determine the owning manager from available updates to avoid spurious errors from other managers
			if updates, err := registry.FetchAllUpdates(cmd.Context()); err == nil {
				for _, u := range updates {
					if strings.EqualFold(u.Id, packageId) || strings.EqualFold(u.Name, packageId) {
						if mgr, ok := registry.GetManager(u.ManagerName); ok {
							err := mgr.UpgradePackage(cmd.Context(), u.Id)
							if err != nil {
								return fmt.Errorf("failed to upgrade package %s with %s: %w", packageId, mgr.Name(), err)
							}
							fmt.Printf("Successfully upgraded package: %s\n", packageId)
							return nil
						}
					}
				}
			}

			// Fallback: iterate through active managers
			var upgraded bool
			var lastErr error
			for _, m := range registry.ActiveManagers(cmd.Context()) {
				err := m.UpgradePackage(cmd.Context(), packageId)
				if err == nil {
					upgraded = true
					fmt.Printf("Successfully upgraded package: %s\n", packageId)
					break
				}
				lastErr = err
			}
			if !upgraded {
				return fmt.Errorf("failed to upgrade package %s: %v", packageId, lastErr)
			}
			return nil
		}

		// Interactive selection mode
		var updates []pm.PackageUpdate
		var err error
		if targetedMgr != nil {
			fmt.Printf("Fetching available updates for %s...\n", targetedMgr.Name())
			updates, err = targetedMgr.GetAvailableUpdates(cmd.Context())
		} else {
			fmt.Println("Fetching available updates...")
			updates, err = registry.FetchAllUpdates(cmd.Context())
		}
		if err != nil {
			return fmt.Errorf("error fetching updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("No updates available.")
			return nil
		}

		var selectedPackages []pm.PackageUpdate

		if upgradeByManager {
			grouped := make(map[string][]pm.PackageUpdate)
			var groupOrder []string
			for _, pkg := range updates {
				mgrName := pkg.ManagerName
				if mgrName == "" {
					mgrName = "other"
				}
				if _, exists := grouped[mgrName]; !exists {
					groupOrder = append(groupOrder, mgrName)
				}
				grouped[mgrName] = append(grouped[mgrName], pkg)
			}

			for _, mgrName := range groupOrder {
				pkgs := grouped[mgrName]
				pluralSuffix := "s"
				if len(pkgs) == 1 {
					pluralSuffix = ""
				}
				fmt.Printf("\n--- %s (%d available update%s) ---\n", mgrName, len(pkgs), pluralSuffix)

				var options []string
				optMap := make(map[string]pm.PackageUpdate)
				maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(pkgs)

				for _, pkg := range pkgs {
					label := formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
					options = append(options, label)
					optMap[label] = pkg
				}

				var selected []string
				prompt := &survey.MultiSelect{
					Message: fmt.Sprintf("Select %s packages to upgrade (Space to select, Enter to confirm/skip):", mgrName),
					Options: options,
				}
				err := survey.AskOne(prompt, &selected)
				if err != nil {
					fmt.Printf("Selection skipped for %s.\n", mgrName)
					continue
				}
				for _, opt := range selected {
					selectedPackages = append(selectedPackages, optMap[opt])
				}
			}
		} else {
			// Unified interactive selection
			var options []string
			optionToUpdate := make(map[string]pm.PackageUpdate)
			maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(updates)

			for _, pkg := range updates {
				label := formatUpdateRow(pkg, maxNameLen, maxIdLen, maxVersionLen, maxAvailLen)
				options = append(options, label)
				optionToUpdate[label] = pkg
			}

			var selectedOptions []string
			prompt := &survey.MultiSelect{
				Message: "Select packages to upgrade (Space to select, Enter to submit, submit empty to cancel):",
				Options: options,
			}
			err = survey.AskOne(prompt, &selectedOptions)
			if err != nil {
				fmt.Println("Selection canceled or failed.")
				return nil
			}

			for _, opt := range selectedOptions {
				selectedPackages = append(selectedPackages, optionToUpdate[opt])
			}
		}

		if len(selectedPackages) == 0 {
			fmt.Println("No packages selected. Canceled.")
			return nil
		}

		selectedPackages = orderSelectedPackages(selectedPackages)

		fmt.Printf("\nUpgrading %d packages...\n", len(selectedPackages))
		hadError := false
		for _, pkg := range selectedPackages {
			fmt.Printf("\n--- Upgrading: %s (%s) ---\n", pkg.Id, pkg.ManagerName)
			mgr, exists := registry.GetManager(pkg.ManagerName)
			if !exists {
				fmt.Fprintf(os.Stderr, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
				hadError = true
				continue
			}
			err := mgr.UpgradePackage(cmd.Context(), pkg.Id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", pkg.Id, err)
				hadError = true
			} else {
				fmt.Printf("Successfully upgraded %s\n", pkg.Id)
			}
		}
		
		fmt.Println("\nAll selected upgrades have completed.")
		if hadError {
			return fmt.Errorf("one or more package upgrades failed")
		}
		return nil
	},
}

func isDeferredUpgradePackage(pkg pm.PackageUpdate) bool {
	lower := strings.ToLower(strings.TrimSpace(pkg.Id))
	return strings.HasPrefix(lower, "microsoft.powershell") ||
		strings.HasPrefix(lower, "microsoft.windowsterminal") ||
		lower == "pwsh" ||
		lower == "powershell" ||
		lower == "windows-terminal"
}

// orderSelectedPackages moves self-updating shell and terminal packages to the end
// of the upgrade queue so that other selected packages complete before the host shell
// or terminal is closed by its installer.
func orderSelectedPackages(pkgs []pm.PackageUpdate) []pm.PackageUpdate {
	ordered := make([]pm.PackageUpdate, len(pkgs))
	copy(ordered, pkgs)
	sort.SliceStable(ordered, func(i, j int) bool {
		return !isDeferredUpgradePackage(ordered[i]) && isDeferredUpgradePackage(ordered[j])
	})
	return ordered
}

func initSurveyTemplates() {
	survey.MultiSelectQuestionTemplate = `
{{- define "option"}}
    {{- if eq .SelectedIndex .CurrentIndex }}{{color .Config.Icons.SelectFocus.Format }}{{ .Config.Icons.SelectFocus.Text }}{{color "reset"}}{{else}} {{end}}
    {{- if index .Checked .CurrentOpt.Index }}{{color .Config.Icons.MarkedOption.Format }} {{ .Config.Icons.MarkedOption.Text }} {{else}}{{color .Config.Icons.UnmarkedOption.Format }} {{ .Config.Icons.UnmarkedOption.Text }} {{end}}
    {{- color "reset"}}
    {{- " "}}{{- .CurrentOpt.Value}}{{ if ne ($.GetDescription .CurrentOpt) "" }} - {{color "cyan"}}{{ $.GetDescription .CurrentOpt }}{{color "reset"}}{{end}}
{{end}}
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}
  {{- range $ix, $opt := .Options}}
    {{- if index $.Checked $ix}}
      {{- "\n  "}}{{- color "cyan"}}{{$opt}}{{color "reset"}}
    {{- end}}
  {{- end}}
  {{- "\n"}}
{{- else }}
	{{- "  "}}{{- color "cyan"}}[Use arrows to move, space to select,{{- if not .Config.RemoveSelectAll }} <right> to all,{{end}}{{- if not .Config.RemoveSelectNone }} <left> to none,{{end}} type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- template "option" $.IterateOption $ix $option}}
  {{- end}}
{{- end}}`
}


func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().BoolVarP(&upgradeAll, "all", "a", false, "Upgrade all available packages")
	upgradeCmd.Flags().StringVarP(&upgradeManager, "manager", "m", "", "Target a specific package manager (winget, scoop, choco)")
	upgradeCmd.Flags().BoolVar(&upgradeByManager, "by-manager", false, "Upgrade packages grouped by package manager")
	initSurveyTemplates()
}

