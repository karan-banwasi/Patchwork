package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/psresource"
	"github.com/karan-banwasi/patchwork/internal/winget"
	"github.com/spf13/cobra"
)

type upgradeOptions struct {
	all              bool
	manager          string
	byManager        bool
	dryRun           bool
	includePinned    bool
	force            bool
	acceptAgreements bool
	trustRepository  bool
	noRefresh        bool
	timeout          time.Duration
}

type upgradeResult struct {
	pkg      pm.PackageUpdate
	status   string
	duration time.Duration
	err      error
}

// printUpgradeSummaryTo renders a post-upgrade summary table of succeeded, failed, and skipped packages to out.
func printUpgradeSummaryTo(out io.Writer, results []upgradeResult) {
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

	fmt.Fprintln(out, "\n=== Upgrade Summary ===")
	headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%s\n", maxIdLen, maxMgrLen, maxStatusLen)
	fmt.Fprintf(out, headerFmt, "Package ID", "Manager", "Status", "Duration")
	fmt.Fprintf(out, headerFmt, strings.Repeat("-", maxIdLen), strings.Repeat("-", maxMgrLen), strings.Repeat("-", maxStatusLen), "--------")
	for _, r := range results {
		durStr := r.duration.Round(100 * time.Millisecond).String()
		if r.duration == 0 {
			durStr = "-"
		}
		fmt.Fprintf(out, headerFmt, r.pkg.Id, r.pkg.ManagerName, r.status, durStr)
	}
	fmt.Fprintf(out, "\nTotal: %d | Succeeded: %d | Failed: %d | Skipped: %d\n", len(results), succeeded, failed, skipped)
}

func configureManagerOptions(reg *pm.Registry, opts upgradeOptions) {
	for _, m := range reg.All() {
		switch mgr := m.(type) {
		case *winget.WingetManager:
			mgr.AcceptAgreements = opts.acceptAgreements
		case *psresource.PSResourceManager:
			mgr.TrustRepository = opts.trustRepository
		}
	}
}

func splitPinnedTargets(pkgs []pm.PackageUpdate, ignorePins bool) (allowed []pm.PackageUpdate, skippedResults []upgradeResult) {
	if ignorePins {
		return pkgs, nil
	}
	cfg, err := config.Load()
	if err != nil || len(cfg.PinnedPackages) == 0 {
		return pkgs, nil
	}
	for _, pkg := range pkgs {
		if cfg.IsPinnedForManager(pkg.ManagerName, pkg.Id, pkg.Name) {
			skippedResults = append(skippedResults, upgradeResult{
				pkg:    pkg,
				status: "Skipped (pinned)",
			})
			continue
		}
		allowed = append(allowed, pkg)
	}
	return allowed, skippedResults
}

func newUpgradeCmd() *cobra.Command {
	opts := upgradeOptions{
		acceptAgreements: true,
	}

	cmd := &cobra.Command{
		Use:               "upgrade [package-id...]",
		Short:             "Upgrade available packages",
		Long:              `Upgrade one or more specific packages by providing their IDs, or upgrade all packages using the --all flag.`,
		ValidArgsFunction: completeAvailablePackageIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.all && len(args) > 0 {
				return fmt.Errorf("--all and a package ID are mutually exclusive")
			}
			if opts.byManager && len(args) > 0 {
				return fmt.Errorf("--by-manager and a package ID are mutually exclusive (cannot specify package IDs when using --by-manager)")
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			registry := getDefaultRegistry()
			configureRegistryIO(registry, cmd.InOrStdin(), out, errOut)
			configureManagerOptions(registry, opts)
			if opts.timeout > 0 {
				registry.FetchTimeout = opts.timeout
			}
			registry.SkipRefresh = opts.noRefresh

			var targetedMgr pm.Manager
			if opts.manager != "" {
				mgr, exists := registry.GetManager(opts.manager)
				if !exists {
					return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
				}
				if !mgr.IsAvailable(cmd.Context()) {
					return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
				}
				targetedMgr = mgr
			}

			ignorePins := opts.includePinned || opts.force

			if opts.all {
				var updates []pm.PackageUpdate
				var warnings []error
				var err error
				if targetedMgr != nil {
					fmt.Fprintf(out, "Fetching available updates for %s...\n", targetedMgr.Name())
					cctx, cancel := context.WithTimeout(cmd.Context(), registry.FetchTimeout)
					defer cancel()
					if !opts.noRefresh {
						if refresher, ok := targetedMgr.(pm.Refresher); ok {
							_ = refresher.Refresh(cctx)
						}
					}
					updates, err = targetedMgr.GetAvailableUpdates(cctx)
				} else {
					fmt.Fprintln(out, "Fetching available updates across package managers...")
					updates, warnings, err = registry.FetchAllUpdatesWithWarnings(cmd.Context())
				}
				if err != nil {
					return fmt.Errorf("error fetching updates: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintf(errOut, "Warning: %v\n", w)
				}

				var pinnedCount int
				if !ignorePins {
					updates, pinnedCount = filterOutPinned(updates)
				}
				if len(updates) == 0 {
					if pinnedCount > 0 {
						fmt.Fprintf(out, "No packages to upgrade (%d pinned package(s) skipped).\n", pinnedCount)
					} else {
						fmt.Fprintln(out, "All packages are up to date.")
					}
					return nil
				}

				ordered := orderSelectedPackagesWithRegistry(registry, updates)
				if opts.dryRun {
					fmt.Fprintf(out, "[Dry Run] Would upgrade %d package(s):\n", len(ordered))
					var results []upgradeResult
					for _, pkg := range ordered {
						results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
					}
					printUpgradeSummaryTo(out, results)
					return nil
				}

				fmt.Fprintf(out, "\nUpgrading %d package(s)...\n", len(ordered))
				results, hadErr := executePackageUpgrades(cmd, registry, ordered)
				printUpgradeSummaryTo(out, results)
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
					allowed, skippedResults := splitPinnedTargets(pkgs, ignorePins)
					for _, sr := range skippedResults {
						fmt.Fprintf(errOut, "Warning: package %q is pinned; skipping (pass --include-pinned or --force to override)\n", sr.pkg.Id)
					}
					if len(allowed) == 0 {
						printUpgradeSummaryTo(out, skippedResults)
						return fmt.Errorf("package %q is pinned (use --force or --include-pinned to upgrade)", strings.Join(args, ", "))
					}
					ordered := orderSelectedPackagesWithRegistry(registry, allowed)
					if opts.dryRun {
						results := append([]upgradeResult{}, skippedResults...)
						for _, pkg := range ordered {
							fmt.Fprintf(out, "[Dry Run] Would upgrade package %s using %s.\n", pkg.Id, targetedMgr.Name())
							results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
						}
						printUpgradeSummaryTo(out, results)
						return nil
					}
					execResults, hadErr := executePackageUpgrades(cmd, registry, ordered)
					results := append(skippedResults, execResults...)
					printUpgradeSummaryTo(out, results)
					if hadErr {
						return fmt.Errorf("one or more package upgrades failed")
					}
					return nil
				}

				fmt.Fprintf(out, "Locating %d package(s) across package managers...\n", len(args))

				updates, warnings, err := registry.FetchAllUpdatesWithWarnings(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to check available updates: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintf(errOut, "Warning: %v\n", w)
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
					fmt.Fprintf(errOut, "Warning: no pending update found for %q (skipping)\n", nf)
					results = append(results, upgradeResult{
						pkg:    pm.PackageUpdate{Name: nf, Id: nf, ManagerName: "unknown"},
						status: "Skipped",
					})
				}

				allowed, pinnedSkipped := splitPinnedTargets(matched, ignorePins)
				for _, sr := range pinnedSkipped {
					fmt.Fprintf(errOut, "Warning: package %q is pinned; skipping (pass --include-pinned or --force to override)\n", sr.pkg.Id)
				}
				results = append(results, pinnedSkipped...)

				if len(allowed) == 0 {
					printUpgradeSummaryTo(out, results)
					return fmt.Errorf("package %q is pinned (use --force or --include-pinned to upgrade)", strings.Join(args, ", "))
				}

				ordered := orderSelectedPackagesWithRegistry(registry, allowed)
				if opts.dryRun {
					for _, u := range ordered {
						fmt.Fprintf(out, "[Dry Run] Would upgrade package %s (%s -> %s) using %s.\n", u.Id, u.Version, u.AvailableVersion, u.ManagerName)
						results = append(results, upgradeResult{pkg: u, status: "Dry-Run"})
					}
					printUpgradeSummaryTo(out, results)
					return nil
				}

				execResults, hadErr := executePackageUpgrades(cmd, registry, ordered)
				results = append(results, execResults...)
				printUpgradeSummaryTo(out, results)
				if hadErr {
					return fmt.Errorf("one or more package upgrades failed")
				}
				return nil
			}

			// Interactive selection mode
			initSurveyTemplates()

			var updates []pm.PackageUpdate
			var warnings []error
			var err error
			if targetedMgr != nil {
				fmt.Fprintf(out, "Fetching available updates for %s...\n", targetedMgr.Name())
				cctx, cancel := context.WithTimeout(cmd.Context(), registry.FetchTimeout)
				defer cancel()
				if !opts.noRefresh {
					if refresher, ok := targetedMgr.(pm.Refresher); ok {
						_ = refresher.Refresh(cctx)
					}
				}
				updates, err = targetedMgr.GetAvailableUpdates(cctx)
			} else {
				fmt.Fprintln(out, "Fetching available updates...")
				updates, warnings, err = registry.FetchAllUpdatesWithWarnings(cmd.Context())
			}
			if err != nil {
				return fmt.Errorf("error fetching updates: %w", err)
			}
			for _, w := range warnings {
				fmt.Fprintf(errOut, "Warning: %v\n", w)
			}

			var pinnedCount int
			if !ignorePins {
				updates, pinnedCount = filterOutPinned(updates)
			}

			if len(updates) == 0 {
				if pinnedCount > 0 {
					fmt.Fprintf(out, "No updates available (%d pinned package(s) skipped).\n", pinnedCount)
				} else {
					fmt.Fprintln(out, "No updates available.")
				}
				return nil
			}

			var selectedPackages []pm.PackageUpdate

			if opts.byManager {
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
					fmt.Fprintf(out, "\n--- %s (%d available update%s) ---\n", mgrName, len(pkgs), pluralSuffix)

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
						fmt.Fprintf(out, "Selection skipped for %s.\n", mgrName)
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
					fmt.Fprintln(out, "Selection canceled or failed.")
					return nil
				}

				for _, opt := range selectedOptions {
					selectedPackages = append(selectedPackages, optionToUpdate[opt])
				}
			}

			if len(selectedPackages) == 0 {
				fmt.Fprintln(out, "No packages selected. Canceled.")
				return nil
			}

			selectedPackages = orderSelectedPackagesWithRegistry(registry, selectedPackages)

			if opts.dryRun {
				fmt.Fprintf(out, "\n[Dry Run] Would upgrade %d selected package(s):\n", len(selectedPackages))
				var results []upgradeResult
				for _, pkg := range selectedPackages {
					results = append(results, upgradeResult{pkg: pkg, status: "Dry-Run"})
				}
				printUpgradeSummaryTo(out, results)
				return nil
			}

			fmt.Fprintf(out, "\nUpgrading %d packages...\n", len(selectedPackages))
			results, hadError := executePackageUpgrades(cmd, registry, selectedPackages)
			printUpgradeSummaryTo(out, results)
			if hadError {
				return fmt.Errorf("one or more package upgrades failed")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.all, "all", "a", false, "Upgrade all available packages")
	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Target a specific package manager")
	cmd.Flags().BoolVar(&opts.byManager, "by-manager", false, "Upgrade packages grouped by package manager")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Preview packages that would be upgraded without installing")
	cmd.Flags().BoolVar(&opts.includePinned, "include-pinned", false, "Upgrade packages even if they are pinned")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force upgrade of pinned packages")
	cmd.Flags().BoolVar(&opts.acceptAgreements, "accept-agreements", true, "Automatically accept winget source and package license agreements")
	cmd.Flags().BoolVar(&opts.trustRepository, "trust-repository", false, "Pass -TrustRepository when updating PowerShell PSResource modules")
	cmd.Flags().BoolVar(&opts.noRefresh, "no-refresh", false, "Skip bucket/repository refresh before checking updates")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", pm.DefaultFetchTimeout, "Per-manager timeout when querying updates")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}

func executePackageUpgrades(cmd *cobra.Command, registry *pm.Registry, pkgs []pm.PackageUpdate) ([]upgradeResult, bool) {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	var results []upgradeResult
	hadError := false

	for i := 0; i < len(pkgs); i++ {
		pkg := pkgs[i]
		mgr, exists := registry.GetManager(pkg.ManagerName)
		if !exists {
			fmt.Fprintf(errOut, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
			results = append(results, upgradeResult{pkg: pkg, status: "Failed", err: fmt.Errorf("manager not registered")})
			hadError = true
			continue
		}

		if err := mgr.ValidateID(pkg.Id); err != nil {
			fmt.Fprintf(errOut, "Error upgrading %s: %v\n", pkg.Id, err)
			results = append(results, upgradeResult{pkg: pkg, status: "Failed", err: err})
			hadError = true
			continue
		}

		// If the manager implements BulkUpgrader (e.g. choco for single-prompt UAC elevation),
		// batch consecutive valid packages for the same manager.
		if bulk, ok := mgr.(pm.BulkUpgrader); ok {
			var batchPkgs []pm.PackageUpdate
			var batchIDs []string
			j := i
			for ; j < len(pkgs) && strings.EqualFold(pkgs[j].ManagerName, pkg.ManagerName); j++ {
				if vErr := mgr.ValidateID(pkgs[j].Id); vErr != nil {
					break
				}
				batchPkgs = append(batchPkgs, pkgs[j])
				batchIDs = append(batchIDs, pkgs[j].Id)
			}
			if len(batchPkgs) > 1 {
				fmt.Fprintf(out, "\n--- Upgrading %d packages (%s): %s ---\n", len(batchIDs), mgr.Name(), strings.Join(batchIDs, ", "))
				start := time.Now()
				err := bulk.UpgradePackages(cmd.Context(), batchIDs)
				dur := time.Since(start) / time.Duration(len(batchPkgs))
				for _, bp := range batchPkgs {
					if err != nil {
						fmt.Fprintf(errOut, "Error upgrading %s: %v\n", bp.Id, err)
						results = append(results, upgradeResult{pkg: bp, status: "Failed", duration: dur, err: err})
						hadError = true
					} else {
						fmt.Fprintf(out, "Successfully upgraded %s\n", bp.Id)
						results = append(results, upgradeResult{pkg: bp, status: "Succeeded", duration: dur})
					}
				}
				i = j - 1
				continue
			}
		}

		fmt.Fprintf(out, "\n--- Upgrading: %s (%s) ---\n", pkg.Id, pkg.ManagerName)
		start := time.Now()
		err := mgr.UpgradePackage(cmd.Context(), pkg.Id)
		dur := time.Since(start)
		if err != nil {
			fmt.Fprintf(errOut, "Error upgrading %s: %v\n", pkg.Id, err)
			results = append(results, upgradeResult{pkg: pkg, status: "Failed", duration: dur, err: err})
			hadError = true
		} else {
			fmt.Fprintf(out, "Successfully upgraded %s\n", pkg.Id)
			results = append(results, upgradeResult{pkg: pkg, status: "Succeeded", duration: dur})
		}
	}
	return results, hadError
}

func isDeferredUpgradePackage(pkg pm.PackageUpdate) bool {
	return (*pm.Registry)(nil).ShouldDefer(pkg)
}

func orderSelectedPackagesWithRegistry(reg *pm.Registry, pkgs []pm.PackageUpdate) []pm.PackageUpdate {
	ordered := make([]pm.PackageUpdate, len(pkgs))
	copy(ordered, pkgs)
	sort.SliceStable(ordered, func(i, j int) bool {
		return !reg.ShouldDefer(ordered[i]) && reg.ShouldDefer(ordered[j])
	})
	return ordered
}

// orderSelectedPackages moves self-updating shell and terminal packages to the end
// of the upgrade queue so that other selected packages complete before the host shell
// or terminal is closed by its installer.
func orderSelectedPackages(pkgs []pm.PackageUpdate) []pm.PackageUpdate {
	return orderSelectedPackagesWithRegistry(getDefaultRegistry(), pkgs)
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
