package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
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
	var uResults []upgrade.Result
	for _, r := range results {
		uResults = append(uResults, upgrade.Result{
			Package:  r.pkg,
			Status:   upgrade.Status(r.status),
			Duration: r.duration,
			Error:    r.err,
		})
	}
	upgrade.NewSummary(uResults).Render(out)
}

func maybePrintWingetAgreementsNotice(out io.Writer, pkgs []pm.PackageUpdate, acceptAgreements bool) {
	if !acceptAgreements {
		return
	}
	for _, p := range pkgs {
		if strings.EqualFold(p.ManagerName, "winget") {
			fmt.Fprintln(out, "Notice: Automatically accepting Winget package and source agreements (--accept-agreements=true).")
			fmt.Fprintln(out, "        Pass --accept-agreements=false to review agreements manually.")
			return
		}
	}
}

func splitPinnedTargets(pkgs []pm.PackageUpdate, ignorePins bool) (allowed []pm.PackageUpdate, skippedResults []upgradeResult, err error) {
	allowedPkgs, skipped, err := upgrade.FilterPins(pkgs, ignorePins)
	if err != nil {
		return nil, nil, err
	}
	for _, s := range skipped {
		skippedResults = append(skippedResults, upgradeResult{
			pkg:      s.Package,
			status:   string(s.Status),
			duration: s.Duration,
			err:      s.Error,
		})
	}
	return allowedPkgs, skippedResults, nil
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
			if !cmd.Flags().Changed("accept-agreements") {
				if cfg, err := config.Load(); err == nil && cfg != nil && cfg.AcceptAgreements != nil {
					opts.acceptAgreements = *cfg.AcceptAgreements
				}
			}
			registry.ConfigureOptions(pm.UpgradeOptions{
				AcceptAgreements: opts.acceptAgreements,
				TrustRepository:  opts.trustRepository,
			})
			if opts.timeout > 0 {
				registry.FetchTimeout = opts.timeout
			}
			registry.SkipRefresh = opts.noRefresh

			ignorePins := opts.includePinned || opts.force

			var plan *upgrade.Plan

			if opts.all {
				var filter []string
				if opts.manager != "" {
					mgr, exists := registry.GetManager(opts.manager)
					if !exists {
						return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
					}
					if !mgr.IsAvailable(cmd.Context()) {
						return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
					}
					filter = append(filter, opts.manager)
					fmt.Fprintf(out, "Fetching available updates for %s...\n", mgr.Name())
				} else {
					fmt.Fprintln(out, "Fetching available updates across package managers...")
				}

				updates, warnings, err := registry.FetchUpdates(cmd.Context(), filter...)
				if err != nil {
					return fmt.Errorf("error fetching updates: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintf(errOut, "Warning: %v\n", w)
				}

				allowed, skipped, err := upgrade.FilterPins(updates, ignorePins)
				if err != nil {
					return fmt.Errorf("cannot verify pinned packages: %w (use --force or --include-pinned to override)", err)
				}
				if len(allowed) == 0 {
					if len(skipped) > 0 {
						fmt.Fprintf(out, "No packages to upgrade (%d pinned package(s) skipped).\n", len(skipped))
					} else {
						fmt.Fprintln(out, "All packages are up to date.")
					}
					return nil
				}
				plan = upgrade.NewPlan(registry, allowed, skipped)
			} else if len(args) > 0 {
				if opts.manager != "" {
					mgr, exists := registry.GetManager(opts.manager)
					if !exists {
						return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
					}
					if !mgr.IsAvailable(cmd.Context()) {
						return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
					}
					var pkgs []pm.PackageUpdate
					for _, packageId := range args {
						pkgs = append(pkgs, pm.PackageUpdate{Name: packageId, Id: packageId, ManagerName: mgr.Name()})
					}
					allowed, skipped, err := upgrade.FilterPins(pkgs, ignorePins)
					if err != nil {
						return err
					}
					for _, sr := range skipped {
						fmt.Fprintf(errOut, "Warning: package %q is pinned; skipping (pass --include-pinned or --force to override)\n", sr.Package.Id)
					}
					if len(allowed) == 0 {
						upgrade.NewSummary(skipped).Render(out)
						return fmt.Errorf("package %q is pinned (use --force or --include-pinned to upgrade)", strings.Join(args, ", "))
					}
					plan = upgrade.NewPlan(registry, allowed, skipped)
				} else {
					fmt.Fprintf(out, "Locating %d package(s) across package managers...\n", len(args))
					updates, warnings, err := registry.FetchUpdates(cmd.Context())
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

					var notFoundSkipped []upgrade.Result
					for _, nf := range notFound {
						fmt.Fprintf(errOut, "Warning: no pending update found for %q (skipping)\n", nf)
						notFoundSkipped = append(notFoundSkipped, upgrade.Result{
							Package: pm.PackageUpdate{Name: nf, Id: nf, ManagerName: "unknown"},
							Status:  upgrade.StatusSkipped,
						})
					}

					allowed, pinnedSkipped, err := upgrade.FilterPins(matched, ignorePins)
					if err != nil {
						return err
					}
					for _, sr := range pinnedSkipped {
						fmt.Fprintf(errOut, "Warning: package %q is pinned; skipping (pass --include-pinned or --force to override)\n", sr.Package.Id)
					}
					allSkipped := append(notFoundSkipped, pinnedSkipped...)
					if len(allowed) == 0 {
						upgrade.NewSummary(allSkipped).Render(out)
						return fmt.Errorf("package %q is pinned (use --force or --include-pinned to upgrade)", strings.Join(args, ", "))
					}
					plan = upgrade.NewPlan(registry, allowed, allSkipped)
				}
			} else {
				// Interactive selection mode
				initSurveyTemplates()

				var filter []string
				if opts.manager != "" {
					mgr, exists := registry.GetManager(opts.manager)
					if !exists {
						return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
					}
					if !mgr.IsAvailable(cmd.Context()) {
						return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
					}
					filter = append(filter, opts.manager)
					fmt.Fprintf(out, "Fetching available updates for %s...\n", mgr.Name())
				} else {
					fmt.Fprintln(out, "Fetching available updates...")
				}

				updates, warnings, err := registry.FetchUpdates(cmd.Context(), filter...)
				if err != nil {
					return fmt.Errorf("error fetching updates: %w", err)
				}
				for _, w := range warnings {
					fmt.Fprintf(errOut, "Warning: %v\n", w)
				}

				allowed, skipped, err := upgrade.FilterPins(updates, ignorePins)
				if err != nil {
					return fmt.Errorf("cannot verify pinned packages: %w (use --force or --include-pinned to override)", err)
				}
				if len(allowed) == 0 {
					if len(skipped) > 0 {
						fmt.Fprintf(out, "No updates available (%d pinned package(s) skipped).\n", len(skipped))
					} else {
						fmt.Fprintln(out, "No updates available.")
					}
					return nil
				}

				var selectedPackages []pm.PackageUpdate

				if opts.byManager {
					grouped := make(map[string][]pm.PackageUpdate)
					var groupOrder []string
					for _, pkg := range allowed {
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
					maxNameLen, maxIdLen, maxVersionLen, maxAvailLen := getDefaultPackageColumnWidths(allowed)

					for _, pkg := range allowed {
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

				plan = upgrade.NewPlan(registry, selectedPackages, nil)
			}

			if plan == nil || plan.IsEmpty() {
				return nil
			}

			maybePrintWingetAgreementsNotice(out, plan.Targets, opts.acceptAgreements)

			if opts.dryRun {
				if len(args) > 0 && opts.manager != "" {
					for _, pkg := range plan.Targets {
						fmt.Fprintf(out, "[Dry Run] Would upgrade package %s using %s.\n", pkg.Id, opts.manager)
					}
				} else if len(args) > 0 {
					for _, u := range plan.Targets {
						fmt.Fprintf(out, "[Dry Run] Would upgrade package %s (%s -> %s) using %s.\n", u.Id, u.Version, u.AvailableVersion, u.ManagerName)
					}
				} else if opts.all {
					fmt.Fprintf(out, "[Dry Run] Would upgrade %d package(s):\n", len(plan.Targets))
				} else {
					fmt.Fprintf(out, "\n[Dry Run] Would upgrade %d selected package(s):\n", len(plan.Targets))
				}
				summary := plan.DryRunSummary()
				summary.Render(out)
				return nil
			}

			if opts.all || len(args) > 0 {
				fmt.Fprintf(out, "\nUpgrading %d package(s)...\n", len(plan.Targets))
			} else {
				fmt.Fprintf(out, "\nUpgrading %d packages...\n", len(plan.Targets))
			}
			executor := upgrade.NewExecutor(registry, out, errOut)
			summary := executor.Execute(cmd.Context(), plan)
			summary.Render(out)
			if summary.HasFailures() {
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
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "Per-manager timeout when querying updates (0 uses manager defaults)")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}

func executePackageUpgrades(cmd *cobra.Command, registry *pm.Registry, pkgs []pm.PackageUpdate) ([]upgradeResult, bool) {
	plan := &upgrade.Plan{Targets: pkgs}
	executor := upgrade.NewExecutor(registry, cmd.OutOrStdout(), cmd.ErrOrStderr())
	summary := executor.Execute(cmd.Context(), plan)
	var results []upgradeResult
	for _, r := range summary.Results {
		results = append(results, upgradeResult{
			pkg:      r.Package,
			status:   string(r.Status),
			duration: r.Duration,
			err:      r.Error,
		})
	}
	return results, summary.HasFailures()
}

func orderSelectedPackagesWithRegistry(reg *pm.Registry, pkgs []pm.PackageUpdate) []pm.PackageUpdate {
	return upgrade.OrderTargets(reg, pkgs)
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
