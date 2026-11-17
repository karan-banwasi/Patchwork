package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/profile"
	"github.com/spf13/cobra"
)

type importOptions struct {
	manager      string
	dryRun       bool
	skipExisting bool
	json         bool
}

type importResultItem struct {
	Manager string `json:"manager"`
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type importJSONSummary struct {
	Total     int `json:"total"`
	Installed int `json:"installed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	DryRun    int `json:"would_install,omitempty"`
}

type importJSONOutput struct {
	SchemaVersion string             `json:"schema_version"`
	DryRun        bool               `json:"dry_run"`
	Results       []importResultItem `json:"results"`
	PinsRestored  int                `json:"pins_restored"`
	Summary       importJSONSummary  `json:"summary"`
}

func newImportCmd() *cobra.Command {
	opts := importOptions{
		skipExisting: true,
	}

	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import packages and pins from a machine profile",
		Long:  `Read a Patchfile (or JSON profile) from a file or stdin ('-'), restore configured pins, and install missing packages.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			inputPath := "Patchfile"
			if len(args) > 0 {
				inputPath = args[0]
			}

			var (
				rawData []byte
				readErr error
			)
			if inputPath == "-" {
				rawData, readErr = io.ReadAll(cmd.InOrStdin())
			} else {
				rawData, readErr = os.ReadFile(inputPath)
			}
			if readErr != nil {
				return fmt.Errorf("failed to read profile %q: %w", inputPath, readErr)
			}

			prof, err := profile.ParsePatchfile(rawData)
			if err != nil {
				return fmt.Errorf("failed to parse profile %q: %w", inputPath, err)
			}

			registry := getDefaultRegistry()
			execOut := out
			if opts.json {
				execOut = io.Discard
			}
			configureRegistryIO(registry, cmd.InOrStdin(), execOut, errOut)

			var targetMgrName string
			if opts.manager != "" {
				mgr, exists := registry.GetManager(opts.manager)
				if !exists {
					return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
				}
				if registry.IsDisabled(mgr.Name()) {
					return fmt.Errorf("package manager %q is disabled in configuration", mgr.Name())
				}
				if !mgr.IsAvailable(ctx) {
					return fmt.Errorf("package manager %q is not installed or available on this system", opts.manager)
				}
				targetMgrName = mgr.Name()
			}

			resolveMgrName := func(raw string) string {
				if m, ok := registry.GetManager(raw); ok {
					return m.Name()
				}
				return strings.ToLower(strings.TrimSpace(raw))
			}

			var packages []profile.ProfilePackage
			for _, pkg := range prof.Packages {
				if targetMgrName != "" && !strings.EqualFold(resolveMgrName(pkg.Manager), targetMgrName) {
					continue
				}
				packages = append(packages, pkg)
			}

			var pins []config.PinEntry
			for _, pin := range prof.Pins {
				if targetMgrName != "" && pin.Manager != "" && !strings.EqualFold(resolveMgrName(pin.Manager), targetMgrName) {
					continue
				}
				pins = append(pins, pin)
			}

			// Preload installed packages per needed manager when --skip-existing is enabled.
			installedByMgr := make(map[string]map[string]bool)
			if opts.skipExisting {
				checkedMgrs := make(map[string]bool)
				for _, pkg := range packages {
					mgr, exists := registry.GetManager(pkg.Manager)
					if !exists {
						continue
					}
					canon := strings.ToLower(mgr.Name())
					if checkedMgrs[canon] {
						continue
					}
					checkedMgrs[canon] = true
					if registry.IsDisabled(mgr.Name()) || !mgr.IsAvailable(ctx) {
						continue
					}
					lister, ok := mgr.(pm.Lister)
					if !ok {
						continue
					}
					cctx, cancel := context.WithTimeout(ctx, registry.ManagerTimeout(mgr))
					installed, listErr := lister.ListInstalled(cctx)
					cancel()
					if listErr != nil {
						if !opts.json {
							fmt.Fprintf(errOut, "Warning: [%s] failed to list installed packages for --skip-existing: %v\n", mgr.Name(), listErr)
						}
						continue
					}
					idSet := make(map[string]bool, len(installed))
					for _, inst := range installed {
						idSet[strings.ToLower(inst.ID)] = true
					}
					installedByMgr[canon] = idSet
				}
			}

			// Restore pins if present.
			pinsRestored := 0
			if len(pins) > 0 {
				if opts.dryRun {
					pinsRestored = len(pins)
				} else {
					cfg, err := config.Load()
					if err != nil {
						return fmt.Errorf("failed to load configuration to restore pins: %w", err)
					}
					for _, pin := range pins {
						if cfg.AddPinEntry(pin) {
							pinsRestored++
						}
					}
					if pinsRestored > 0 {
						if err := cfg.Save(); err != nil {
							return fmt.Errorf("failed to save restored pins: %w", err)
						}
					}
				}
			}

			var (
				results        = make([]importResultItem, 0, len(packages))
				installedCount int
				skippedCount   int
				failedCount    int
				dryRunCount    int
			)

			for _, pkg := range packages {
				mgr, exists := registry.GetManager(pkg.Manager)
				if !exists {
					failedCount++
					results = append(results, importResultItem{
						Manager: pkg.Manager,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "failed",
						Error:   fmt.Sprintf("unknown package manager %q", pkg.Manager),
					})
					continue
				}

				canonName := mgr.Name()
				if registry.IsDisabled(canonName) {
					failedCount++
					results = append(results, importResultItem{
						Manager: canonName,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "failed",
						Error:   fmt.Sprintf("package manager %q is disabled in configuration", canonName),
					})
					continue
				}
				if !mgr.IsAvailable(ctx) {
					failedCount++
					results = append(results, importResultItem{
						Manager: canonName,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "failed",
						Error:   fmt.Sprintf("package manager %q is not installed or available on this system", canonName),
					})
					continue
				}

				if opts.skipExisting {
					if idSet, ok := installedByMgr[strings.ToLower(canonName)]; ok && idSet[strings.ToLower(pkg.ID)] {
						skippedCount++
						results = append(results, importResultItem{
							Manager: canonName,
							ID:      pkg.ID,
							Version: pkg.Version,
							Status:  "skipped",
						})
						continue
					}
				}

				installer, ok := mgr.(pm.Installer)
				if !ok {
					failedCount++
					results = append(results, importResultItem{
						Manager: canonName,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "failed",
						Error:   fmt.Sprintf("package manager %q does not support installing packages", canonName),
					})
					continue
				}

				if opts.dryRun {
					dryRunCount++
					results = append(results, importResultItem{
						Manager: canonName,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "would_install",
					})
					continue
				}

				if !opts.json {
					if pkg.Version != "" {
						fmt.Fprintf(out, "Installing %s (%s) via %s...\n", pkg.ID, pkg.Version, canonName)
					} else {
						fmt.Fprintf(out, "Installing %s via %s...\n", pkg.ID, canonName)
					}
				}

				if err := installer.InstallPackage(ctx, pkg.ID, pkg.Version); err != nil {
					failedCount++
					results = append(results, importResultItem{
						Manager: canonName,
						ID:      pkg.ID,
						Version: pkg.Version,
						Status:  "failed",
						Error:   err.Error(),
					})
					continue
				}

				installedCount++
				results = append(results, importResultItem{
					Manager: canonName,
					ID:      pkg.ID,
					Version: pkg.Version,
					Status:  "installed",
				})
			}

			if opts.json {
				payload := importJSONOutput{
					SchemaVersion: "1.0.0",
					DryRun:        opts.dryRun,
					Results:       results,
					PinsRestored:  pinsRestored,
					Summary: importJSONSummary{
						Total:     len(results),
						Installed: installedCount,
						Skipped:   skippedCount,
						Failed:    failedCount,
						DryRun:    dryRunCount,
					},
				}
				enc := json.NewEncoder(out)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					return err
				}
			} else {
				renderImportSummary(out, results, pinsRestored, opts.dryRun, installedCount, skippedCount, failedCount, dryRunCount)
			}

			if failedCount > 0 {
				return exitcodes.New(exitcodes.ExitUpgradeFailure, fmt.Sprintf("%d package(s) failed to install", failedCount))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Import packages only for a specific package manager")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Preview packages and pins that would be imported without modifying the system")
	cmd.Flags().BoolVar(&opts.skipExisting, "skip-existing", true, "Skip packages that are already installed")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output import results as JSON")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}

func renderImportSummary(out io.Writer, results []importResultItem, pinsRestored int, dryRun bool, installed, skipped, failed, wouldInstall int) {
	if len(results) == 0 && pinsRestored == 0 {
		fmt.Fprintln(out, "Profile contains no packages or pins to import.")
		return
	}

	if len(results) > 0 {
		maxMgr := len("Manager")
		maxID := len("Package ID")
		maxVer := len("Version")
		maxStatus := len("Status")

		for _, r := range results {
			if n := utf8.RuneCountInString(r.Manager); n > maxMgr {
				maxMgr = n
			}
			if n := utf8.RuneCountInString(r.ID); n > maxID {
				maxID = n
			}
			ver := r.Version
			if ver == "" {
				ver = "latest"
			}
			if n := utf8.RuneCountInString(ver); n > maxVer {
				maxVer = n
			}
			if n := utf8.RuneCountInString(r.Status); n > maxStatus {
				maxStatus = n
			}
		}

		if dryRun {
			fmt.Fprintln(out, "\n=== Import Dry-Run Summary ===")
		} else {
			fmt.Fprintln(out, "\n=== Import Summary ===")
		}

		rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n", maxMgr, maxID, maxVer, maxStatus)
		fmt.Fprintf(out, rowFmt, "Manager", "Package ID", "Version", "Status", "Details")
		fmt.Fprintf(out, rowFmt, strings.Repeat("-", maxMgr), strings.Repeat("-", maxID), strings.Repeat("-", maxVer), strings.Repeat("-", maxStatus), "-------")
		for _, r := range results {
			ver := r.Version
			if ver == "" {
				ver = "latest"
			}
			details := r.Error
			if details == "" {
				details = "-"
			}
			fmt.Fprintf(out, rowFmt, r.Manager, r.ID, ver, r.Status, details)
		}
	}

	if dryRun {
		fmt.Fprintf(out, "\nTotal: %d | Would install: %d | Skipped: %d | Failed: %d | Pins to restore: %d\n",
			len(results), wouldInstall, skipped, failed, pinsRestored)
	} else {
		fmt.Fprintf(out, "\nTotal: %d | Installed: %d | Skipped: %d | Failed: %d | Pins restored: %d\n",
			len(results), installed, skipped, failed, pinsRestored)
	}
}
