package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

// Status represents the state of a package upgrade or plan item.
type Status string

const (
	StatusSucceeded       Status = "Succeeded"
	StatusFailed          Status = "Failed"
	StatusSkipped         Status = "Skipped"
	StatusSkippedPinned   Status = "Skipped (pinned)"
	StatusSkippedExcluded Status = "Skipped (excluded)"
	StatusSkippedLevel    Status = "Skipped (level)"
	StatusDryRun          Status = "Dry-Run"
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

// FilterPins checks packages against config pins (supporting version constraints and expiry).
func FilterPins(pkgs []pm.PackageUpdate, ignorePins bool) (allowed []pm.PackageUpdate, skipped []Result, err error) {
	if ignorePins {
		return pkgs, nil, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot verify pinned packages: %w (use --force or --include-pinned to override)", err)
	}
	if len(cfg.PinnedPackages) == 0 && len(cfg.Pins) == 0 {
		return pkgs, nil, nil
	}
	for _, pkg := range pkgs {
		pin, violated := cfg.MatchPin(pkg.ManagerName, pkg.Id, pkg.Name, pkg.AvailableVersion)
		if violated {
			status := StatusSkippedPinned
			if pin != nil && (pin.Version != "" || pin.Reason != "") {
				var desc []string
				if pin.Version != "" {
					desc = append(desc, pin.Version)
				}
				if pin.Reason != "" {
					desc = append(desc, pin.Reason)
				}
				status = Status(fmt.Sprintf("Skipped (pinned: %s)", strings.Join(desc, " - ")))
			}
			skipped = append(skipped, Result{
				Package: pkg,
				Status:  status,
			})
			continue
		}
		allowed = append(allowed, pkg)
	}
	return allowed, skipped, nil
}

// Executor coordinates package upgrades using managers from the registry.
type Executor struct {
	Registry    *pm.Registry
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	Concurrency int
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

type indexedTarget struct {
	origIdx int
	pkg     pm.PackageUpdate
}

type managerGroup struct {
	key   string
	items []indexedTarget
}

// Execute runs all targets in the plan, executing distinct non-deferred managers in parallel
// (bounded by Concurrency when > 0; 1 = serial; <= 0 = parallel across all managers),
// keeping execution within each manager sequential (preserving BulkUpgrader/DetailedBulkUpgrader),
// buffering per-manager output so logs never interleave, preserving plan.Targets order in Summary.Results,
// and running deferred packages in Phase 2 after all non-deferred managers finish.
func (e *Executor) Execute(ctx context.Context, plan *Plan) Summary {
	var results []Result
	if plan != nil {
		results = append(results, plan.Skipped...)
	}
	if plan == nil || len(plan.Targets) == 0 {
		return NewSummary(results)
	}

	stdout := e.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := e.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	phase1Map := make(map[string]int)
	var phase1Groups []managerGroup
	phase2Map := make(map[string]int)
	var phase2Groups []managerGroup

	for i, pkg := range plan.Targets {
		mgrKey := strings.ToLower(strings.TrimSpace(pkg.ManagerName))
		if e.Registry != nil {
			if mgr, ok := e.Registry.GetManager(pkg.ManagerName); ok {
				mgrKey = strings.ToLower(mgr.Name())
			}
		}

		deferred := e.Registry != nil && e.Registry.ShouldDefer(pkg)
		if deferred {
			idx, exists := phase2Map[mgrKey]
			if !exists {
				idx = len(phase2Groups)
				phase2Map[mgrKey] = idx
				phase2Groups = append(phase2Groups, managerGroup{key: mgrKey})
			}
			phase2Groups[idx].items = append(phase2Groups[idx].items, indexedTarget{origIdx: i, pkg: pkg})
		} else {
			idx, exists := phase1Map[mgrKey]
			if !exists {
				idx = len(phase1Groups)
				phase1Map[mgrKey] = idx
				phase1Groups = append(phase1Groups, managerGroup{key: mgrKey})
			}
			phase1Groups[idx].items = append(phase1Groups[idx].items, indexedTarget{origIdx: i, pkg: pkg})
		}
	}

	targetResults := make([]Result, len(plan.Targets))

	// Phase 1: Non-deferred managers
	if len(phase1Groups) > 0 {
		if e.Concurrency == 1 || len(phase1Groups) == 1 {
			for _, g := range phase1Groups {
				e.executeManagerGroup(ctx, g.items, stdout, stderr, stdout, stderr, targetResults)
			}
		} else {
			limit := len(phase1Groups)
			if e.Concurrency > 0 && e.Concurrency < limit {
				limit = e.Concurrency
			}
			sem := make(chan struct{}, limit)
			var wg sync.WaitGroup
			var ioMu sync.Mutex

			for _, g := range phase1Groups {
				wg.Add(1)
				go func(grp managerGroup) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					var outBuf, errBuf bytes.Buffer
					e.executeManagerGroup(ctx, grp.items, &outBuf, &errBuf, stdout, stderr, targetResults)

					ioMu.Lock()
					defer ioMu.Unlock()
					if outBuf.Len() > 0 {
						_, _ = outBuf.WriteTo(stdout)
					}
					if errBuf.Len() > 0 {
						_, _ = errBuf.WriteTo(stderr)
					}
				}(g)
			}
			wg.Wait()
		}
	}

	// Phase 2: Deferred packages run sequentially after all non-deferred managers finish
	for _, g := range phase2Groups {
		e.executeManagerGroup(ctx, g.items, stdout, stderr, stdout, stderr, targetResults)
	}

	results = append(results, targetResults...)
	return NewSummary(results)
}

func (e *Executor) executeManagerGroup(
	ctx context.Context,
	items []indexedTarget,
	out io.Writer,
	errOut io.Writer,
	restoreOut io.Writer,
	restoreErr io.Writer,
	targetResults []Result,
) {
	for i := 0; i < len(items); i++ {
		item := items[i]
		pkg := item.pkg

		var mgr pm.Manager
		var exists bool
		if e.Registry != nil {
			mgr, exists = e.Registry.GetManager(pkg.ManagerName)
		}
		if !exists {
			fmt.Fprintf(errOut, "Error: package manager %q not registered for package %s\n", pkg.ManagerName, pkg.Id)
			targetResults[item.origIdx] = Result{
				Package: pkg,
				Status:  StatusFailed,
				Error:   fmt.Errorf("manager %q not registered", pkg.ManagerName),
			}
			continue
		}

		if ioCfg, ok := mgr.(pm.IOConfigurable); ok {
			ioCfg.SetIO(e.Stdin, out, errOut)
		}

		if err := mgr.ValidateID(pkg.Id); err != nil {
			fmt.Fprintf(errOut, "Error upgrading %s: %v\n", pkg.Id, err)
			targetResults[item.origIdx] = Result{
				Package: pkg,
				Status:  StatusFailed,
				Error:   err,
			}
			continue
		}

		// Check for DetailedBulkUpgrader first to report per-package results
		if detailed, ok := mgr.(pm.DetailedBulkUpgrader); ok {
			var batchItems []indexedTarget
			var batchIDs []string
			j := i
			for ; j < len(items); j++ {
				if vErr := mgr.ValidateID(items[j].pkg.Id); vErr != nil {
					break
				}
				batchItems = append(batchItems, items[j])
				batchIDs = append(batchIDs, items[j].pkg.Id)
			}
			if len(batchItems) > 1 {
				fmt.Fprintf(out, "\n--- Upgrading %d packages (%s): %s ---\n", len(batchIDs), mgr.Name(), strings.Join(batchIDs, ", "))
				start := time.Now()
				bulkResults, err := detailed.UpgradePackagesDetailed(ctx, batchIDs)
				dur := time.Since(start) / time.Duration(len(batchItems))
				if err != nil && len(bulkResults) == 0 {
					for _, bi := range batchItems {
						fmt.Fprintf(errOut, "Error upgrading %s: %v\n", bi.pkg.Id, err)
						targetResults[bi.origIdx] = Result{
							Package:  bi.pkg,
							Status:   StatusFailed,
							Duration: dur,
							Error:    err,
						}
					}
				} else {
					resMap := make(map[string]pm.BulkPackageResult)
					for _, br := range bulkResults {
						resMap[strings.ToLower(br.ID)] = br
					}
					for _, bi := range batchItems {
						br, found := resMap[strings.ToLower(bi.pkg.Id)]
						if found && br.Succeeded {
							fmt.Fprintf(out, "Successfully upgraded %s\n", bi.pkg.Id)
							targetResults[bi.origIdx] = Result{
								Package:  bi.pkg,
								Status:   StatusSucceeded,
								Duration: dur,
							}
						} else {
							pkgErr := err
							if found && br.Error != nil {
								pkgErr = br.Error
							} else if pkgErr == nil {
								pkgErr = fmt.Errorf("package upgrade failed")
							}
							fmt.Fprintf(errOut, "Error upgrading %s: %v\n", bi.pkg.Id, pkgErr)
							targetResults[bi.origIdx] = Result{
								Package:  bi.pkg,
								Status:   StatusFailed,
								Duration: dur,
								Error:    pkgErr,
							}
						}
					}
				}
				i = j - 1
				continue
			}
		}

		// Fallback for standard BulkUpgrader
		if bulk, ok := mgr.(pm.BulkUpgrader); ok {
			var batchItems []indexedTarget
			var batchIDs []string
			j := i
			for ; j < len(items); j++ {
				if vErr := mgr.ValidateID(items[j].pkg.Id); vErr != nil {
					break
				}
				batchItems = append(batchItems, items[j])
				batchIDs = append(batchIDs, items[j].pkg.Id)
			}
			if len(batchItems) > 1 {
				fmt.Fprintf(out, "\n--- Upgrading %d packages (%s): %s ---\n", len(batchIDs), mgr.Name(), strings.Join(batchIDs, ", "))
				start := time.Now()
				err := bulk.UpgradePackages(ctx, batchIDs)
				dur := time.Since(start) / time.Duration(len(batchItems))
				for _, bi := range batchItems {
					if err != nil {
						fmt.Fprintf(errOut, "Error upgrading %s: %v\n", bi.pkg.Id, err)
						targetResults[bi.origIdx] = Result{
							Package:  bi.pkg,
							Status:   StatusFailed,
							Duration: dur,
							Error:    err,
						}
					} else {
						fmt.Fprintf(out, "Successfully upgraded %s\n", bi.pkg.Id)
						targetResults[bi.origIdx] = Result{
							Package:  bi.pkg,
							Status:   StatusSucceeded,
							Duration: dur,
						}
					}
				}
				i = j - 1
				continue
			}
		}

		fmt.Fprintf(out, "\n--- Upgrading: %s (%s) ---\n", pkg.Id, pkg.ManagerName)
		start := time.Now()
		err := mgr.UpgradePackage(ctx, pkg.Id)
		dur := time.Since(start)
		if err != nil {
			fmt.Fprintf(errOut, "Error upgrading %s: %v\n", pkg.Id, err)
			targetResults[item.origIdx] = Result{
				Package:  pkg,
				Status:   StatusFailed,
				Duration: dur,
				Error:    err,
			}
		} else {
			fmt.Fprintf(out, "Successfully upgraded %s\n", pkg.Id)
			targetResults[item.origIdx] = Result{
				Package:  pkg,
				Status:   StatusSucceeded,
				Duration: dur,
			}
		}
	}

	// Restore original registry streams if IOConfigurable was redirected to buffers.
	if len(items) > 0 && e.Registry != nil {
		if mgr, ok := e.Registry.GetManager(items[0].pkg.ManagerName); ok {
			if ioCfg, ok := mgr.(pm.IOConfigurable); ok {
				ioCfg.SetIO(e.Stdin, restoreOut, restoreErr)
			}
		}
	}
}
