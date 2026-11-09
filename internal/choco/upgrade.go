package choco

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.DetailedBulkUpgrader = (*ChocoManager)(nil)
	_ pm.BulkUpgrader         = (*ChocoManager)(nil)

	chocoSuccessRegex     = regexp.MustCompile(`(?i)(?:the\s+upgrade\s+of\s+([^\s]+)\s+was\s+successful|([^\s]+)\s+v[^\s]+\s+was\s+installed\s+successfully|([^\s]+)\s+v[^\s]+\s+is\s+the\s+latest\s+version\s+available)`)
	chocoFailBodyRegex    = regexp.MustCompile(`(?i)the\s+upgrade\s+of\s+([^\s]+)\s+was\s+NOT\s+successful`)
	chocoFailureLineRegex = regexp.MustCompile(`^\s*-\s+([^\s]+)(?:\s+\(exited\s+(-?\d+)\))?\s*-\s*(.*)$`)
)

// ParseChocoUpgradeOutput parses Chocolatey stdout to determine per-package upgrade results.
func ParseChocoUpgradeOutput(output string, ids []string) ([]pm.BulkPackageResult, bool) {
	if len(ids) == 0 {
		return nil, false
	}

	succeededMap := make(map[string]bool)
	failedMap := make(map[string]error)

	lines := strings.Split(output, "\n")
	inFailuresSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.EqualFold(trimmed, "Failures") {
			inFailuresSection = true
			continue
		}

		if inFailuresSection {
			if strings.HasPrefix(trimmed, "-") {
				if m := chocoFailureLineRegex.FindStringSubmatch(trimmed); len(m) >= 4 {
					pkgID := m[1]
					errMsg := strings.TrimSpace(m[3])
					if m[2] != "" {
						failedMap[strings.ToLower(pkgID)] = fmt.Errorf("exited %s: %s", m[2], errMsg)
					} else {
						failedMap[strings.ToLower(pkgID)] = fmt.Errorf("%s", errMsg)
					}
					continue
				}
			} else if len(trimmed) > 0 && !strings.Contains(trimmed, " ") {
				inFailuresSection = false
			}
		}

		if m := chocoFailBodyRegex.FindStringSubmatch(trimmed); len(m) >= 2 {
			pkgID := strings.ToLower(m[1])
			if _, exists := failedMap[pkgID]; !exists {
				failedMap[pkgID] = fmt.Errorf("upgrade was not successful")
			}
			continue
		}

		if m := chocoSuccessRegex.FindStringSubmatch(trimmed); len(m) >= 2 {
			pkgID := m[1]
			if pkgID == "" {
				pkgID = m[2]
			}
			if pkgID == "" {
				pkgID = m[3]
			}
			if pkgID != "" {
				succeededMap[strings.ToLower(pkgID)] = true
			}
		}
	}

	if len(succeededMap) == 0 && len(failedMap) == 0 {
		return nil, false
	}

	results := make([]pm.BulkPackageResult, len(ids))
	for i, id := range ids {
		key := strings.ToLower(id)
		if err, failed := failedMap[key]; failed {
			results[i] = pm.BulkPackageResult{
				ID:        id,
				Succeeded: false,
				Error:     err,
			}
		} else if succeededMap[key] {
			results[i] = pm.BulkPackageResult{
				ID:        id,
				Succeeded: true,
			}
		} else if len(failedMap) == 0 && len(succeededMap) > 0 {
			// If all reported items succeeded and no failures occurred
			results[i] = pm.BulkPackageResult{
				ID:        id,
				Succeeded: true,
			}
		} else {
			results[i] = pm.BulkPackageResult{
				ID:        id,
				Succeeded: false,
				Error:     fmt.Errorf("upgrade status inconclusive"),
			}
		}
	}

	return results, true
}

// UpgradePackage upgrades a specific package by its ID using choco upgrade -y.
func (c *ChocoManager) UpgradePackage(ctx context.Context, id string) error {
	return c.UpgradePackages(ctx, []string{id})
}

// UpgradePackagesDetailed upgrades one or more Chocolatey packages and returns individual outcomes.
func (c *ChocoManager) UpgradePackagesDetailed(ctx context.Context, ids []string) ([]pm.BulkPackageResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	for _, id := range ids {
		if err := c.ValidateID(id); err != nil {
			return nil, err
		}
	}

	stdout := c.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.stderr
	if stderr == nil {
		stderr = io.Discard
	}

	args := make([]string, 0, len(ids)+3)
	args = append(args, "upgrade")
	args = append(args, ids...)
	args = append(args, "-y", "--no-progress")

	runner := c.runner
	_, isExec := runner.(pm.ExecRunner)
	_, isExecPtr := runner.(*pm.ExecRunner)
	isProductionRunner := runner == nil || isExec || isExecPtr

	var exe string
	if isProductionRunner && !isElevated() {
		var err error
		exe, err = trustedChocoPath()
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(stdout, "Chocolatey requires Administrator privileges to upgrade %s. Requesting UAC elevation...\n", strings.Join(ids, ", "))
		elevErr := runElevated(ctx, exe, args...)
		if elevErr != nil {
			return nil, elevErr
		}
		results := make([]pm.BulkPackageResult, len(ids))
		for i, id := range ids {
			results[i] = pm.BulkPackageResult{ID: id, Succeeded: true}
		}
		return results, nil
	}

	if runner == nil {
		runner = pm.ExecRunner{}
	}
	var err error
	exe, err = chocoPath()
	if err != nil {
		if !isProductionRunner {
			exe = "choco"
		} else {
			return nil, err
		}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	runErr := runner.Run(ctx, exe, args, c.stdin, io.MultiWriter(stdout, &stdoutBuf), io.MultiWriter(stderr, &stderrBuf))

	if results, ok := ParseChocoUpgradeOutput(stdoutBuf.String(), ids); ok {
		return results, nil
	}

	if runErr != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("choco upgrade failed: %w (%s)", runErr, stderrStr)
		}
		return nil, fmt.Errorf("choco upgrade failed: %w", runErr)
	}

	results := make([]pm.BulkPackageResult, len(ids))
	for i, id := range ids {
		results[i] = pm.BulkPackageResult{ID: id, Succeeded: true}
	}
	return results, nil
}

// UpgradePackages upgrades one or more Chocolatey packages in a single invocation
// so that non-elevated runs trigger at most one UAC prompt.
func (c *ChocoManager) UpgradePackages(ctx context.Context, ids []string) error {
	results, err := c.UpgradePackagesDetailed(ctx, ids)
	if err != nil {
		return err
	}
	var failed []string
	for _, r := range results {
		if !r.Succeeded {
			if r.Error != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", r.ID, r.Error))
			} else {
				failed = append(failed, r.ID)
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("chocolatey upgrade failed for: %s", strings.Join(failed, ", "))
	}
	return nil
}

// UpgradePackage is a package-level helper for convenience.
func UpgradePackage(ctx context.Context, id string) error {
	return NewChocoManager().UpgradePackage(ctx, id)
}

// IsProductionRunner reports whether the runner is a production runner (nil or ExecRunner).
func IsProductionRunner(r pm.Runner) bool {
	if r == nil {
		return true
	}
	_, isExec := r.(pm.ExecRunner)
	_, isExecPtr := r.(*pm.ExecRunner)
	return isExec || isExecPtr
}

func isProductionRunner(r pm.Runner) bool {
	return IsProductionRunner(r)
}
