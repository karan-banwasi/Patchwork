package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/profile"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	output          string
	manager         string
	includeVersions bool
	includePins     bool
	json            bool
}

func newExportCmd() *cobra.Command {
	opts := exportOptions{
		includePins: true,
	}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export installed packages and pins to a machine profile",
		Long:  `Export installed packages across active package managers and configured pins into a portable Patchfile DSL or JSON profile.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			registry := getDefaultRegistry()
			configureRegistryIO(registry, cmd.InOrStdin(), io.Discard, errOut)

			var targets []pm.Manager
			var targetMgrName string

			if opts.manager != "" {
				mgr, err := resolveManager(ctx, registry, opts.manager, true, true)
				if err != nil {
					return err
				}
				if _, ok := mgr.(pm.Lister); !ok {
					return fmt.Errorf("package manager %q does not support listing installed packages", mgr.Name())
				}
				targets = append(targets, mgr)
				targetMgrName = mgr.Name()
			} else {
				for _, mgr := range registry.ActiveManagers(ctx) {
					if _, ok := mgr.(pm.Lister); ok {
						targets = append(targets, mgr)
					}
				}
			}

			type listResult struct {
				mgrName string
				pkgs    []pm.InstalledPackage
				err     error
			}

			results := make([]listResult, len(targets))
			var wg sync.WaitGroup
			for i, mgr := range targets {
				wg.Add(1)
				go func(idx int, m pm.Manager) {
					defer wg.Done()
					lister := m.(pm.Lister)
					cctx, cancel := context.WithTimeout(ctx, registry.ManagerTimeout(m))
					defer cancel()
					pkgs, err := lister.ListInstalled(cctx)
					results[idx] = listResult{
						mgrName: m.Name(),
						pkgs:    pkgs,
						err:     err,
					}
				}(i, mgr)
			}
			wg.Wait()

			var (
				collected []profile.ProfilePackage
				mgrErrs   []error
			)
			for _, res := range results {
				if res.err != nil {
					mgrErrs = append(mgrErrs, fmt.Errorf("[%s] %w", res.mgrName, res.err))
					continue
				}
				for _, pkg := range res.pkgs {
					mgrName := pkg.ManagerName
					if mgrName == "" {
						mgrName = res.mgrName
					}
					ver := ""
					if opts.includeVersions {
						ver = pkg.Version
					}
					collected = append(collected, profile.ProfilePackage{
						Manager: strings.ToLower(mgrName),
						ID:      pkg.ID,
						Name:    pkg.Name,
						Version: ver,
					})
				}
			}

			if len(targets) > 0 && len(mgrErrs) == len(targets) {
				return fmt.Errorf("failed to export installed packages: %w", errors.Join(mgrErrs...))
			}
			for _, w := range mgrErrs {
				fmt.Fprintf(errOut, "Warning: %v\n", w)
			}

			sort.Slice(collected, func(i, j int) bool {
				if collected[i].Manager != collected[j].Manager {
					return collected[i].Manager < collected[j].Manager
				}
				return strings.ToLower(collected[i].ID) < strings.ToLower(collected[j].ID)
			})

			var pins []config.PinEntry
			if opts.includePins {
				if cfg, err := config.Load(); err == nil && cfg != nil {
					for _, p := range cfg.Pins {
						if targetMgrName != "" && p.Manager != "" {
							resolved := p.Manager
							if m, ok := registry.GetManager(p.Manager); ok {
								resolved = m.Name()
							}
							if !strings.EqualFold(resolved, targetMgrName) {
								continue
							}
						}
						pins = append(pins, p)
					}
				}
			}

			prof := profile.Profile{
				SchemaVersion: profile.SchemaVersion,
				CreatedAt:     time.Now().UTC(),
				Packages:      collected,
				Pins:          pins,
			}

			var data []byte
			if opts.json {
				var err error
				data, err = profile.MarshalJSON(prof)
				if err != nil {
					return fmt.Errorf("failed to marshal profile JSON: %w", err)
				}
			} else {
				data = []byte(profile.FormatPatchfile(prof))
			}

			if opts.output != "" {
				if err := profile.WriteAtomic(opts.output, data, 0644); err != nil {
					return fmt.Errorf("failed to write profile to %q: %w", opts.output, err)
				}
				if !opts.json {
					fmt.Fprintf(out, "Exported %d package(s) and %d pin(s) to %s.\n", len(collected), len(pins), opts.output)
				}
				return nil
			}

			_, err := out.Write(data)
			return err
		},
	}

	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Write exported profile to a file instead of stdout")
	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Export packages from a specific package manager")
	cmd.Flags().BoolVar(&opts.includeVersions, "include-versions", false, "Include installed package versions in the exported profile")
	cmd.Flags().BoolVar(&opts.includePins, "include-pins", true, "Include configured package pins in the exported profile")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output exported profile in JSON format")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}
