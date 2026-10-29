package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

type doctorRow struct {
	name    string
	status  string
	version string
	path    string
}

// findPWInstallationsOnPath returns all distinct pw.exe / pw binary paths found across PATH entries in order.
func findPWInstallationsOnPath(pathEnv string) []string {
	exeNames := []string{"pw.exe"}
	if runtime.GOOS != "windows" {
		exeNames = []string{"pw", "pw.exe"}
	}
	seen := make(map[string]bool)
	var found []string
	for _, dir := range filepath.SplitList(pathEnv) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		for _, name := range exeNames {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			clean := filepath.Clean(candidate)
			key := clean
			if runtime.GOOS == "windows" {
				key = strings.ToLower(clean)
			}
			if !seen[key] {
				seen[key] = true
				found = append(found, clean)
			}
		}
	}
	return found
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Aliases: []string{"status"},
		Short:   "Show system diagnostics and detected package managers",
		Long:    `Inspect Patchwork's environment, configuration paths, and all supported package managers on your system.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			reg := getDefaultRegistry()

			fmt.Fprintf(out, "Patchwork %s (%s/%s)\n\n", version, runtime.GOOS, runtime.GOARCH)

			cfgPath, _ := config.ConfigPath()
			cfg, _ := config.Load()
			pinnedCount := 0
			if cfg != nil {
				pinnedCount = len(cfg.PinnedPackages)
			}
			fmt.Fprintf(out, "Configuration:\n")
			fmt.Fprintf(out, "  Config File:     %s\n", cfgPath)
			fmt.Fprintf(out, "  Update Cache:    %s\n", config.CachePath())
			fmt.Fprintf(out, "  Pinned Packages: %d\n", pinnedCount)

			pwPaths := findPWInstallationsOnPath(os.Getenv("PATH"))
			if len(pwPaths) > 0 {
				fmt.Fprintf(out, "  PATH Binaries:   %s\n", strings.Join(pwPaths, ", "))
				if len(pwPaths) > 1 {
					fmt.Fprintf(out, "  Warning: Multiple pw installations found on PATH (%d). %s takes precedence.\n", len(pwPaths), pwPaths[0])
				}
			}
			fmt.Fprintln(out)

			var rows []doctorRow
			maxName := 7
			maxStatus := 10
			maxVer := 7

			for _, mgr := range reg.All() {
				name := mgr.Name()
				avail := mgr.IsAvailable(cmd.Context())
				status := "Not Found"
				binPath := "-"
				ver := "-"
				if avail {
					status = "Available"
					if loc, ok := mgr.(pm.Locator); ok {
						if p, err := loc.Path(); err == nil && p != "" {
							binPath = p
						}
						if v, err := loc.Version(cmd.Context()); err == nil && v != "" {
							ver = v
						} else if v != "" {
							ver = v
						}
					}
				}
				if n := utf8.RuneCountInString(name); n > maxName {
					maxName = n
				}
				if n := utf8.RuneCountInString(status); n > maxStatus {
					maxStatus = n
				}
				if n := utf8.RuneCountInString(ver); n > maxVer {
					maxVer = n
				}
				rows = append(rows, doctorRow{
					name:    name,
					status:  status,
					version: ver,
					path:    binPath,
				})
			}

			fmt.Fprintf(out, "Package Managers:\n")
			hdrFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%s\n", maxName, maxStatus, maxVer)
			fmt.Fprintf(out, hdrFmt, "Manager", "Status", "Version", "Executable")
			fmt.Fprintf(out, hdrFmt, strings.Repeat("-", maxName), strings.Repeat("-", maxStatus), strings.Repeat("-", maxVer), "----------")
			for _, r := range rows {
				fmt.Fprintf(out, hdrFmt, r.name, r.status, r.version, r.path)
			}
			return nil
		},
	}
}
