package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/history"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
	"github.com/spf13/cobra"
)

type rollbackOptions struct {
	last    bool
	runID   string
	to      string
	manager string
	dryRun  bool
	json    bool
}

type rollbackTarget struct {
	Name          string
	ID            string
	Manager       string
	FromVersion   string
	TargetVersion string
}

func newRollbackCmd() *cobra.Command {
	var opts rollbackOptions

	cmd := &cobra.Command{
		Use:   "rollback [package-id...]",
		Short: "Roll back upgraded packages to a previous version",
		Long:  `Roll back one or more packages to their previous version recorded in upgrade history, or to an explicit version with --to.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRollbackFlags(opts, args); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			registry := getDefaultRegistry()
			configureRegistryIO(registry, cmd.InOrStdin(), out, errOut)

			var mgrFilter string
			if opts.manager != "" {
				mgr, err := resolveManager(cmd.Context(), registry, opts.manager, false, false)
				if err != nil {
					return err
				}
				mgrFilter = mgr.Name()
			}

			targets, preFailures, err := resolveRollbackTargets(opts, args, mgrFilter)
			if err != nil {
				return err
			}

			if len(targets) == 0 && len(preFailures) == 0 {
				return fmt.Errorf("no packages to roll back")
			}

			// Validate rollback support upfront when targeting a single package explicitly.
			if len(targets) == 1 && len(preFailures) == 0 {
				t := targets[0]
				if err := validateRollbackTarget(registry, t); err != nil {
					return err
				}
			}

			if opts.dryRun {
				return renderRollbackDryRun(out, registry, targets, preFailures, opts.json)
			}

			results := append([]upgrade.Result{}, preFailures...)
			for _, t := range targets {
				pkgMeta := pm.PackageUpdate{
					Name:             t.Name,
					Id:               t.ID,
					Version:          t.FromVersion,
					AvailableVersion: t.TargetVersion,
					ManagerName:      t.Manager,
				}
				if err := validateRollbackTarget(registry, t); err != nil {
					if !opts.json {
						fmt.Fprintf(errOut, "Error rolling back %s (%s): %v\n", t.ID, t.Manager, err)
					}
					results = append(results, upgrade.Result{
						Package: pkgMeta,
						Status:  upgrade.StatusFailed,
						Error:   err,
					})
					continue
				}

				mgr, _ := registry.GetManager(t.Manager)
				if !mgr.IsAvailable(cmd.Context()) {
					availErr := fmt.Errorf("package manager %q is not installed or available on this system", t.Manager)
					if !opts.json {
						fmt.Fprintf(errOut, "Error rolling back %s (%s): %v\n", t.ID, t.Manager, availErr)
					}
					results = append(results, upgrade.Result{
						Package: pkgMeta,
						Status:  upgrade.StatusFailed,
						Error:   availErr,
					})
					continue
				}

				rb := mgr.(pm.Rollbackable)
				if !opts.json {
					fmt.Fprintf(out, "\n--- Rolling back: %s (%s) to %s ---\n", t.ID, t.Manager, t.TargetVersion)
				}
				start := time.Now()
				rbErr := rb.RollbackPackage(cmd.Context(), t.ID, t.TargetVersion)
				dur := time.Since(start)
				if rbErr != nil {
					if !opts.json {
						fmt.Fprintf(errOut, "Error rolling back %s: %v\n", t.ID, rbErr)
					}
					results = append(results, upgrade.Result{
						Package:  pkgMeta,
						Status:   upgrade.StatusFailed,
						Duration: dur,
						Error:    rbErr,
					})
				} else {
					if !opts.json {
						fmt.Fprintf(out, "Successfully rolled back %s to %s\n", t.ID, t.TargetVersion)
					}
					results = append(results, upgrade.Result{
						Package:  pkgMeta,
						Status:   upgrade.StatusSucceeded,
						Duration: dur,
					})
				}
			}

			summary := upgrade.NewSummary(results)
			if opts.json {
				jsonErr := summary.RenderJSON(out, false)
				if summary.HasFailures() {
					return exitcodes.New(exitcodes.ExitUpgradeFailure, "one or more package rollbacks failed")
				}
				return jsonErr
			}

			summary.Render(out)
			if summary.HasFailures() {
				return exitcodes.New(exitcodes.ExitUpgradeFailure, "one or more package rollbacks failed")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.last, "last", false, "Roll back all succeeded packages from the most recent upgrade run")
	cmd.Flags().StringVar(&opts.runID, "run", "", "Roll back all succeeded packages from a specific upgrade run ID")
	cmd.Flags().StringVar(&opts.to, "to", "", "Target version to roll back to (single package only)")
	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Scope rollback to a specific package manager")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Preview rollback actions without modifying installed packages")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output rollback results as JSON")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}

func validateRollbackFlags(opts rollbackOptions, args []string) error {
	if opts.last && opts.runID != "" {
		return fmt.Errorf("--last and --run are mutually exclusive")
	}
	if len(args) > 0 && (opts.last || opts.runID != "") {
		return fmt.Errorf("cannot specify package IDs together with --last or --run")
	}
	if len(args) == 0 && !opts.last && opts.runID == "" {
		return fmt.Errorf("specify at least one package ID, --last, or --run <run-id>")
	}
	if opts.to != "" {
		if opts.last || opts.runID != "" {
			return fmt.Errorf("--to cannot be used with --last or --run")
		}
		if len(args) > 1 {
			return fmt.Errorf("--to can only be used when rolling back a single package")
		}
		if err := pm.ValidateVersion(opts.to); err != nil {
			return err
		}
	}
	return nil
}

func resolveRollbackTargets(opts rollbackOptions, args []string, mgrFilter string) ([]rollbackTarget, []upgrade.Result, error) {
	if opts.last || opts.runID != "" {
		return resolveRunRollbackTargets(opts, mgrFilter)
	}

	var targets []rollbackTarget
	for _, pkgArg := range args {
		if opts.to != "" && mgrFilter != "" {
			// When both -m and --to are explicitly provided, history lookup is optional.
			targets = append(targets, rollbackTarget{
				Name:          pkgArg,
				ID:            pkgArg,
				Manager:       mgrFilter,
				TargetVersion: opts.to,
			})
			continue
		}

		item, _, err := history.FindLatestForPackage(mgrFilter, pkgArg)
		if err != nil {
			return nil, nil, err
		}
		if item == nil {
			if mgrFilter != "" {
				return nil, nil, fmt.Errorf("no successful upgrade history found for package %q in manager %q", pkgArg, mgrFilter)
			}
			return nil, nil, fmt.Errorf("no successful upgrade history found for package %q (specify -m <manager> and --to <version> to roll back manually)", pkgArg)
		}

		targetVer := opts.to
		if targetVer == "" {
			if isUnknownVersion(item.FromVersion) {
				return nil, nil, fmt.Errorf("previous version for package %q (%s) is unknown in history; specify --to <version> explicitly", item.ID, item.Manager)
			}
			targetVer = item.FromVersion
		}

		name := item.Name
		if name == "" {
			name = item.ID
		}
		targets = append(targets, rollbackTarget{
			Name:          name,
			ID:            item.ID,
			Manager:       item.Manager,
			FromVersion:   item.ToVersion,
			TargetVersion: targetVer,
		})
	}
	return targets, nil, nil
}

func resolveRunRollbackTargets(opts rollbackOptions, mgrFilter string) ([]rollbackTarget, []upgrade.Result, error) {
	var entry *history.Entry
	if opts.runID != "" {
		var err error
		entry, err = history.FindEntry(opts.runID)
		if err != nil {
			return nil, nil, err
		}
		if entry == nil {
			return nil, nil, fmt.Errorf("upgrade run %q not found in history", opts.runID)
		}
	} else {
		entries, err := history.LoadAll()
		if err != nil {
			return nil, nil, err
		}
		if len(entries) == 0 {
			return nil, nil, fmt.Errorf("no upgrade history found")
		}
		latest := entries[len(entries)-1]
		entry = &latest
	}

	var targets []rollbackTarget
	var preFailures []upgrade.Result
	for _, item := range entry.Items {
		if !strings.EqualFold(item.Status, string(upgrade.StatusSucceeded)) {
			continue
		}
		if mgrFilter != "" && !strings.EqualFold(item.Manager, mgrFilter) {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.ID
		}
		if isUnknownVersion(item.FromVersion) {
			preFailures = append(preFailures, upgrade.Result{
				Package: pm.PackageUpdate{
					Name:        name,
					Id:          item.ID,
					Version:     item.ToVersion,
					ManagerName: item.Manager,
				},
				Status: upgrade.StatusFailed,
				Error:  fmt.Errorf("previous version for package %q (%s) is unknown in history; specify --to <version> explicitly", item.ID, item.Manager),
			})
			continue
		}
		targets = append(targets, rollbackTarget{
			Name:          name,
			ID:            item.ID,
			Manager:       item.Manager,
			FromVersion:   item.ToVersion,
			TargetVersion: item.FromVersion,
		})
	}

	if len(targets) == 0 && len(preFailures) == 0 {
		return nil, nil, fmt.Errorf("no succeeded packages found in run %q", entry.RunID)
	}
	if len(targets) == 0 && len(preFailures) == 1 {
		return nil, nil, preFailures[0].Error
	}
	return targets, preFailures, nil
}

func validateRollbackTarget(registry *pm.Registry, t rollbackTarget) error {
	mgr, exists := registry.GetManager(t.Manager)
	if !exists {
		return fmt.Errorf("package manager %q is not registered or supported on this platform", t.Manager)
	}
	if _, ok := mgr.(pm.Rollbackable); !ok {
		return fmt.Errorf("package manager %q does not support rollback", mgr.Name())
	}
	if err := mgr.ValidateID(t.ID); err != nil {
		return err
	}
	if err := pm.ValidateVersion(t.TargetVersion); err != nil {
		return err
	}
	return nil
}

func renderRollbackDryRun(out io.Writer, registry *pm.Registry, targets []rollbackTarget, preFailures []upgrade.Result, asJSON bool) error {
	results := append([]upgrade.Result{}, preFailures...)
	for _, t := range targets {
		pkgMeta := pm.PackageUpdate{
			Name:             t.Name,
			Id:               t.ID,
			Version:          t.FromVersion,
			AvailableVersion: t.TargetVersion,
			ManagerName:      t.Manager,
		}
		if err := validateRollbackTarget(registry, t); err != nil {
			results = append(results, upgrade.Result{
				Package: pkgMeta,
				Status:  upgrade.StatusFailed,
				Error:   err,
			})
			continue
		}
		results = append(results, upgrade.Result{
			Package: pkgMeta,
			Status:  upgrade.StatusDryRun,
		})
	}

	summary := upgrade.NewSummary(results)
	if asJSON {
		return summary.RenderJSON(out, true)
	}
	for _, t := range targets {
		fmt.Fprintf(out, "[Dry Run] Would roll back package %s (%s) to version %s.\n", t.ID, t.Manager, t.TargetVersion)
	}
	summary.Render(out)
	return nil
}

func isUnknownVersion(v string) bool {
	trimmed := strings.TrimSpace(v)
	return trimmed == "" || strings.EqualFold(trimmed, "unknown")
}
