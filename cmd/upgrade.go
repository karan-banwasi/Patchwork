package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

var (
	upgradeAll       bool
	upgradeManager   string
	upgradeByManager bool
	upgradeDryRun    bool
)

type upgradeResult struct {
	pkg      pm.PackageUpdate
	status   string
	duration time.Duration
	err      error
}

// printUpgradeSummary renders a post-upgrade summary table of succeeded, failed, and skipped packages.
func printUpgradeSummary(results []upgradeResult) {
	if len(results) == 0 {
		return
	}
	var succeeded, failed, skipped int
	maxIdLen := 10
	maxMgrLen := 7
	maxStatusLen := 9

	for _, r := range results {
		switch r.status {
		case "Succeeded":
			succeeded++
		case "Failed":
			failed++
		default:
			skipped++
		}
		if n := utf8.RuneCountInString(r.pkg.Id); n > maxIdLen {
			maxIdLen = n
		}
		if n := utf8.RuneCountInString(r.pkg.ManagerName); n > maxMgrLen {
			maxMgrLen = n
		}
		if n := utf8.RuneCountInString(r.status); n > maxStatusLen {
			maxStatusLen = n
		}
	}

	fmt.Println("\n=== Upgrade Summary ===")
	headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%s\n", maxIdLen, maxMgrLen, maxStatusLen)
	fmt.Printf(headerFmt, "Package ID", "Manager", "Status", "Duration")
	fmt.Printf(headerFmt, strings.Repeat("-", maxIdLen), strings.Repeat("-", maxMgrLen), strings.Repeat("-", maxStatusLen), "--------")
	for _, r := range results {
		durStr := r.duration.Round(100 * time.Millisecond).String()
		if r.duration == 0 {
			durStr = "-"
		}
		fmt.Printf(headerFmt, r.pkg.Id, r.pkg.ManagerName, r.status, durStr)
	}
	fmt.Printf("\nTotal: %d | Succeeded: %d | Failed: %d | Skipped: %d\n", len(results), succeeded, failed, skipped)
}

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:               "upgrade [package-id...]",
	Short:             "Upgrade available packages",
	Long:              `Upgrade one or more specific packages by providing their IDs, or upgrade all packages using the --all flag.`,
	ValidArgsFunction: completeAvailablePackageIDs,
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
				return fmt.Errorf("unknown package manager %q (supported: %s)", upgradeManager, strings.Join(registry.ManagerNames(), ", "))
			}
			if !mgr.IsAvailable(cmd.Context()) {
				return fmt.Errorf("package manager %q is not installed or available on this system", upgradeManager)
			}
			targetedMgr = mgr
		}

		if upgradeAll {
			var updates []pm.PackageUpdate
			var err error
			if targetedMgr != nil {
				fmt.Printf("Fetching available updates for %s...\n", targetedMgr.Name())
				updates, err = targetedMgr.GetAvailableUpdates(cmd.Context())
			} else {
				fmt.Println("Fetching available updates across package managers...")
				updates, err = registry.FetchAllUpdates(cmd.Context())
			}
			if err != nil {
				return fmt.Errorf("error fetching updates: %w", err)
			}

			var pinnedCount int
			updates, pinnedCount = filterOutPinned(updates)
			if len(updates) == 0 {
				if pinnedCount > 0 {
					fmt.Printf("No packages to upgrade (%d pinned package(s) skipped).\n", pinnedCount)
				} else {
					fmt.Println("All packages are up to date.")
				}
				return nil
			}

			ordered := orderSelectedPackages(updates)
			if upgradeDryRun {
				fmt.Printf("[Dry Run] Would upgrade %d package(s):\n", len(ordered))
				var results []upgradeResult
				for _, pkg := range ordered {
					results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
				}
				printUpgradeSummary(results)
				return nil
			}

			fmt.Printf("\nUpgrading %d package(s)...\n", len(ordered))
			results, hadErr := executePackageUpgrades(cmd, registry, ordered)
			printUpgradeSummary(results)
			if hadErr {
				return fmt.Errorf("one or more package upgrades failed")
			}
			return nil
		}

		if len(args) > 0 {
			if targetedMgr != nil {
				var pkgs []pm.PackageUpdate
				for _, packageId := range args {
					pkgs = append(pkgs, pm.PackageUpdate{Name: packageId, Id: packageId, ManagerName: targetedMgr.Name()})
				}
				ordered := orderSelectedPackages(pkgs)
				if upgradeDryRun {
					for _, pkg := range ordered {
						fmt.Printf("[Dry Run] Would upgrade package %s using %s.\n", pkg.Id, targetedMgr.Name())
					}
					var results []upgradeResult
					for _, pkg := range ordered {
						results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
					}
					printUpgradeSummary(results)
					return nil
				}
				results, hadErr := executePackageUpgrades(cmd, registry, ordered)
				printUpgradeSummary(results)
				if hadErr {
					return fmt.Errorf("one or more package upgrades failed")
				}
				return nil
			}

			fmt.Printf("Locating %d package(s) across package managers...\n", len(args))

			updates, err := registry.FetchAllUpdates(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to check available updates: %w", err)
			}

			var matched []pm.PackageUpdate
			var notFound []string
			for _, packageId := range args {
				found := false
				for _, u := range updates {
					if strings.EqualFold(u.Id, packageId) || strings.EqualFold(u.Name, packageId) {
						matched = append(matched, u)
						found = true
						break
					}
				}
				if !found {
					notFound = append(notFound, packageId)
				}
			}

			if len(matched) == 0 {
				return fmt.Errorf("no pending update found for package %q across active package managers (use -m <manager> to force an upgrade with a specific manager)", strings.Join(notFound, ", "))
			}

			var results []upgradeResult
			for _, nf := range notFound {
				fmt.Fprintf(os.Stderr, "Warning: no pending update found for %q (skipping)\n", nf)
				results = append(results, upgradeResult{
					pkg:    pm.PackageUpdate{Name: nf, Id: nf, ManagerName: "unknown"},
					status: "Skipped",
				})
			}

			ordered := orderSelectedPackages(matched)
			if upgradeDryRun {
				for _, u := range ordered {
					fmt.Printf("[Dry Run] Would upgrade package %s (%s -> %s) using %s.\n", u.Id, u.Version, u.AvailableVersion, u.ManagerName)
					results = append(results, upgradeResult{pkg: u, status: "Dry-Run"})
				}
				printUpgradeSummary(results)
				return nil
			}

			execResults, hadErr := executePackageUpgrades(cmd, registry, ordered)
			results = append(results, execResults...)
			printUpgradeSummary(results)
			if hadErr {
				return fmt.Errorf("one or more package upgrades failed")
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

		var pinnedCount int
		updates, pinnedCount = filterOutPinned(updates)

		if len(updates) == 0 {
			if pinnedCount > 0 {
				fmt.Printf("No updates available (%d pinned package(s) skipped).\n", pinnedCount)
			} else {
				fmt.Println("No updates available.")
			}
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

		if upgradeDryRun {
			fmt.Printf("\n[Dry Run] Would upgrade %d selected package(s):\n", len(selectedPackages))
			var results []upgradeResult
			for _, pkg := range selectedPackages {
				results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
			}
			printUpgradeSummary(results)
			return nil
		}

		fmt.Printf("\nUpgrading %d packages...\n", len(selectedPackages))
		results, hadError := executePackageUpgrades(cmd, registry, selectedPackages)
		printUpgradeSummary(results)
		if hadError {
			return fmt.Errorf("one or more package upgrades failed")
		}
		return nil
	},
}

func executePackageUpgrades(cmd *cobra.Command, registry *pm.Registry, pkgs []pm.PackageUpdate) ([]upgradeResult, bool) {
	var results []upgradeResult
	hadError := false
	for _, pkg := range pkgs {
		fmt.Printf("\n--- Upgrading: %s (%s) ---\n", pkg.Id, pkg.ManagerName)
		mgr, exists := registry.GetManager(pkg.ManagerName)
		if !exists {
			fmt.Fprintf(os.Stderr, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
			results = append(results, upgradeResult{pkg: pkg, status: "Failed", err: fmt.Errorf("manager not registered")})
			hadError = true
			continue
		}
		start := time.Now()
		err := mgr.UpgradePackage(cmd.Context(), pkg.Id)
		dur := time.Since(start)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading %s: %v\n", pkg.Id, err)
			results = append(results, upgradeResult{pkg: pkg, status: "Failed", duration: dur, err: err})
			hadError = true
		} else {
			fmt.Printf("Successfully upgraded %s\n", pkg.Id)
			results = append(results, upgradeResult{pkg: pkg, status: "Succeeded", duration: dur})
		}
	}
	return results, hadError
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
	upgradeCmd.Flags().StringVarP(&upgradeManager, "manager", "m", "", "Target a specific package manager")
	upgradeCmd.Flags().BoolVar(&upgradeByManager, "by-manager", false, "Upgrade packages grouped by package manager")
	upgradeCmd.Flags().BoolVar(&upgradeDryRun, "dry-run", false, "Preview packages that would be upgraded without installing")
	_ = upgradeCmd.RegisterFlagCompletionFunc("manager", completeManagerNames)
	initSurveyTemplates()
}

