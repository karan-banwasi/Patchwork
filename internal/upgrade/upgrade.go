package upgrade

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

// Status represents the state of a package upgrade or plan item.
type Status string

const (
	StatusSucceeded     Status = "Succeeded"
	StatusFailed        Status = "Failed"
	StatusSkipped       Status = "Skipped"
	StatusSkippedPinned Status = "Skipped (pinned)"
	StatusDryRun        Status = "Dry-Run"
)

// Result captures the outcome of an upgrade attempt.
type Result struct {
	Package  pm.PackageUpdate `json:"package"`
	Status   Status           `json:"status"`
	Duration time.Duration    `json:"duration"`
	Error    error            `json:"error,omitempty"`
}

// Summary aggregates upgrade outcomes and formats the output table.
type Summary struct {
	Results   []Result `json:"results"`
	Total     int      `json:"total"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
}

// NewSummary constructs a Summary from a list of upgrade results.
func NewSummary(results []Result) Summary {
	var succeeded, failed, skipped int
	for _, r := range results {
		switch r.Status {
		case StatusSucceeded:
			succeeded++
		case StatusFailed:
			failed++
		default:
			skipped++
		}
	}
	return Summary{
		Results:   results,
		Total:     len(results),
		Succeeded: succeeded,
		Failed:    failed,
		Skipped:   skipped,
	}
}

// HasFailures reports whether any package in the summary failed.
func (s Summary) HasFailures() bool {
	return s.Failed > 0
}

// Render writes a formatted summary table of package results to out.
func (s Summary) Render(out io.Writer) {
	if len(s.Results) == 0 {
		return
	}
	maxIdLen := 10
	maxMgrLen := 7
	maxStatusLen := 9

	for _, r := range s.Results {
		if n := utf8.RuneCountInString(r.Package.Id); n > maxIdLen {
			maxIdLen = n
		}
		if n := utf8.RuneCountInString(r.Package.ManagerName); n > maxMgrLen {
			maxMgrLen = n
		}
		if n := utf8.RuneCountInString(string(r.Status)); n > maxStatusLen {
			maxStatusLen = n
		}
	}

	fmt.Fprintln(out, "\n=== Upgrade Summary ===")
	headerFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%s\n", maxIdLen, maxMgrLen, maxStatusLen)
	fmt.Fprintf(out, headerFmt, "Package ID", "Manager", "Status", "Duration")
	fmt.Fprintf(out, headerFmt, strings.Repeat("-", maxIdLen), strings.Repeat("-", maxMgrLen), strings.Repeat("-", maxStatusLen), "--------")
	for _, r := range s.Results {
		durStr := r.Duration.Round(100 * time.Millisecond).String()
		if r.Duration == 0 {
			durStr = "-"
		}
		fmt.Fprintf(out, headerFmt, r.Package.Id, r.Package.ManagerName, r.Status, durStr)
	}
	fmt.Fprintf(out, "\nTotal: %d | Succeeded: %d | Failed: %d | Skipped: %d\n", s.Total, s.Succeeded, s.Failed, s.Skipped)
}

// Plan encapsulates an ordered sequence of targets and any skipped packages.
type Plan struct {
	Targets []pm.PackageUpdate
	Skipped []Result
}

// NewPlan constructs an upgrade Plan with targets ordered according to registry deferral rules.
func NewPlan(reg *pm.Registry, targets []pm.PackageUpdate, skipped []Result) *Plan {
	return &Plan{
		Targets: OrderTargets(reg, targets),
		Skipped: skipped,
	}
}

// IsEmpty reports whether the plan has no executable targets.
func (p *Plan) IsEmpty() bool {
	return p == nil || (len(p.Targets) == 0 && len(p.Skipped) == 0)
}

// DryRunResults produces the list of Results representing a dry-run preview.
func (p *Plan) DryRunResults() []Result {
	if p == nil {
		return nil
	}
	results := append([]Result{}, p.Skipped...)
	for _, pkg := range p.Targets {
		results = append(results, Result{
			Package: pkg,
			Status:  StatusDryRun,
		})
	}
	return results
}

// DryRunSummary produces a Summary for the dry-run results.
func (p *Plan) DryRunSummary() Summary {
	return NewSummary(p.DryRunResults())
}

// OrderTargets moves deferred packages (e.g. self-updating shells or terminals) to the end.
func OrderTargets(reg *pm.Registry, pkgs []pm.PackageUpdate) []pm.PackageUpdate {
	ordered := make([]pm.PackageUpdate, len(pkgs))
	copy(ordered, pkgs)
	if reg == nil {
		return ordered
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return !reg.ShouldDefer(ordered[i]) && reg.ShouldDefer(ordered[j])
	})
	return ordered
}

// FilterPins checks packages against config.PinnedPackages.
func FilterPins(pkgs []pm.PackageUpdate, ignorePins bool) (allowed []pm.PackageUpdate, skipped []Result, err error) {
	if ignorePins {
		return pkgs, nil, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot verify pinned packages: %w (use --force or --include-pinned to override)", err)
	}
	if len(cfg.PinnedPackages) == 0 {
		return pkgs, nil, nil
	}
	for _, pkg := range pkgs {
		if cfg.IsPinnedForManager(pkg.ManagerName, pkg.Id, pkg.Name) {
			skipped = append(skipped, Result{
				Package: pkg,
				Status:  StatusSkippedPinned,
			})
			continue
		}
		allowed = append(allowed, pkg)
	}
	return allowed, skipped, nil
}

// Executor coordinates package upgrades using managers from the registry.
type Executor struct {
	Registry *pm.Registry
	Stdout   io.Writer
	Stderr   io.Writer
}

// NewExecutor creates a new Executor with specified registry and I/O streams.
func NewExecutor(reg *pm.Registry, stdout, stderr io.Writer) *Executor {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Executor{
		Registry: reg,
		Stdout:   stdout,
		Stderr:   stderr,
	}
}

// Execute runs all targets in the plan, applying BulkUpgrader batching when supported.
func (e *Executor) Execute(ctx context.Context, plan *Plan) Summary {
	var results []Result
	if plan != nil {
		results = append(results, plan.Skipped...)
	}
	if plan == nil || len(plan.Targets) == 0 {
		return NewSummary(results)
	}

	pkgs := plan.Targets
	for i := 0; i < len(pkgs); i++ {
		pkg := pkgs[i]
		mgr, exists := e.Registry.GetManager(pkg.ManagerName)
		if !exists {
			fmt.Fprintf(e.Stderr, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
			results = append(results, Result{
				Package: pkg,
				Status:  StatusFailed,
				Error:   fmt.Errorf("manager %q not registered", pkg.ManagerName),
			})
			continue
		}

		if err := mgr.ValidateID(pkg.Id); err != nil {
			fmt.Fprintf(e.Stderr, "Error upgrading %s: %v\n", pkg.Id, err)
			results = append(results, Result{
				Package: pkg,
				Status:  StatusFailed,
				Error:   err,
			})
			continue
		}

		// Check for DetailedBulkUpgrader first to report per-package results
		if detailed, ok := mgr.(pm.DetailedBulkUpgrader); ok {
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
				fmt.Fprintf(e.Stdout, "\n--- Upgrading %d packages (%s): %s ---\n", len(batchIDs), mgr.Name(), strings.Join(batchIDs, ", "))
				start := time.Now()
				bulkResults, err := detailed.UpgradePackagesDetailed(ctx, batchIDs)
				dur := time.Since(start) / time.Duration(len(batchPkgs))
				if err != nil && len(bulkResults) == 0 {
					for _, bp := range batchPkgs {
						fmt.Fprintf(e.Stderr, "Error upgrading %s: %v\n", bp.Id, err)
						results = append(results, Result{
							Package:  bp,
							Status:   StatusFailed,
							Duration: dur,
							Error:    err,
						})
					}
				} else {
					resMap := make(map[string]pm.BulkPackageResult)
					for _, br := range bulkResults {
						resMap[strings.ToLower(br.ID)] = br
					}
					for _, bp := range batchPkgs {
						br, found := resMap[strings.ToLower(bp.Id)]
						if found && br.Succeeded {
							fmt.Fprintf(e.Stdout, "Successfully upgraded %s\n", bp.Id)
							results = append(results, Result{
								Package:  bp,
								Status:   StatusSucceeded,
								Duration: dur,
							})
						} else {
							pkgErr := err
							if found && br.Error != nil {
								pkgErr = br.Error
							} else if pkgErr == nil {
								pkgErr = fmt.Errorf("package upgrade failed")
							}
							fmt.Fprintf(e.Stderr, "Error upgrading %s: %v\n", bp.Id, pkgErr)
							results = append(results, Result{
								Package:  bp,
								Status:   StatusFailed,
								Duration: dur,
								Error:    pkgErr,
							})
						}
					}
				}
				i = j - 1
				continue
			}
		}

		// Fallback for standard BulkUpgrader
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
				fmt.Fprintf(e.Stdout, "\n--- Upgrading %d packages (%s): %s ---\n", len(batchIDs), mgr.Name(), strings.Join(batchIDs, ", "))
				start := time.Now()
				err := bulk.UpgradePackages(ctx, batchIDs)
				dur := time.Since(start) / time.Duration(len(batchPkgs))
				for _, bp := range batchPkgs {
					if err != nil {
						fmt.Fprintf(e.Stderr, "Error upgrading %s: %v\n", bp.Id, err)
						results = append(results, Result{
							Package:  bp,
							Status:   StatusFailed,
							Duration: dur,
							Error:    err,
						})
					} else {
						fmt.Fprintf(e.Stdout, "Successfully upgraded %s\n", bp.Id)
						results = append(results, Result{
							Package:  bp,
							Status:   StatusSucceeded,
							Duration: dur,
						})
					}
				}
				i = j - 1
				continue
			}
		}

		fmt.Fprintf(e.Stdout, "\n--- Upgrading: %s (%s) ---\n", pkg.Id, pkg.ManagerName)
		start := time.Now()
		err := mgr.UpgradePackage(ctx, pkg.Id)
		dur := time.Since(start)
		if err != nil {
			fmt.Fprintf(e.Stderr, "Error upgrading %s: %v\n", pkg.Id, err)
			results = append(results, Result{
				Package:  pkg,
				Status:   StatusFailed,
				Duration: dur,
				Error:    err,
			})
		} else {
			fmt.Fprintf(e.Stdout, "Successfully upgraded %s\n", pkg.Id)
			results = append(results, Result{
				Package:  pkg,
				Status:   StatusSucceeded,
				Duration: dur,
			})
		}
	}

	return NewSummary(results)
}
