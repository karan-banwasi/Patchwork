package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/spf13/cobra"
)

type doctorRow struct {
	name    string
	status  string
	version string
	path    string
}

func resolveManagerBinaryAndVersion(name string) (string, string) {
	switch name {
	case "winget":
		localAppData := os.Getenv("LOCALAPPDATA")
		var path string
		if localAppData != "" {
			candidate := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
		if path == "" {
			path, _ = exec.LookPath("winget.exe")
		}
		if path == "" {
			return "", ""
		}
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			return path, "unknown"
		}
		return path, strings.TrimSpace(string(out))

	case "scoop":
		for _, envKey := range []string{"SCOOP", "SCOOP_GLOBAL"} {
			if envVal := os.Getenv(envKey); envVal != "" {
				for _, shim := range []string{"scoop.cmd", "scoop.exe"} {
					p := filepath.Join(envVal, "shims", shim)
					if _, err := os.Stat(p); err == nil {
						return p, "installed"
					}
				}
			}
		}
		if up := os.Getenv("USERPROFILE"); up != "" {
			for _, shim := range []string{"scoop.cmd", "scoop.exe"} {
				p := filepath.Join(up, "scoop", "shims", shim)
				if _, err := os.Stat(p); err == nil {
					return p, "installed"
				}
			}
		}
		for _, n := range []string{"scoop.cmd", "scoop.exe", "scoop"} {
			if p, err := exec.LookPath(n); err == nil {
				return p, "installed"
			}
		}
		return "", ""

	case "choco":
		var path string
		if pd := os.Getenv("ProgramData"); pd != "" {
			p := filepath.Join(pd, "chocolatey", "bin", "choco.exe")
			if _, err := os.Stat(p); err == nil {
				path = p
			}
		}
		if path == "" {
			for _, n := range []string{"choco.exe", "choco"} {
				if p, err := exec.LookPath(n); err == nil {
					path = p
					break
				}
			}
		}
		if path == "" {
			return "", ""
		}
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			return path, "unknown"
		}
		return path, strings.TrimSpace(string(out))

	case "npm":
		for _, n := range []string{"npm.cmd", "npm.exe", "npm"} {
			if p, err := exec.LookPath(n); err == nil {
				out, err := exec.Command(p, "--version").Output()
				if err != nil {
					return p, "unknown"
				}
				return p, strings.TrimSpace(string(out))
			}
		}
		return "", ""

	case "dotnet":
		for _, n := range []string{"dotnet.exe", "dotnet"} {
			if p, err := exec.LookPath(n); err == nil {
				out, err := exec.Command(p, "--version").Output()
				if err != nil {
					return p, "unknown"
				}
				return p, strings.TrimSpace(string(out))
			}
		}
		return "", ""

	case "uv":
		for _, n := range []string{"uv.exe", "uv"} {
			if p, err := exec.LookPath(n); err == nil {
				out, err := exec.Command(p, "--version").Output()
				if err != nil {
					return p, "unknown"
				}
				return p, strings.TrimPrefix(strings.TrimSpace(string(out)), "uv ")
			}
		}
		return "", ""

	case "cargo":
		for _, n := range []string{"cargo.exe", "cargo"} {
			if p, err := exec.LookPath(n); err == nil {
				out, err := exec.Command(p, "--version").Output()
				if err != nil {
					return p, "unknown"
				}
				return p, strings.TrimPrefix(strings.TrimSpace(string(out)), "cargo ")
			}
		}
		return "", ""

	case "psresource":
		for _, n := range []string{"pwsh.exe", "pwsh", "powershell.exe"} {
			if p, err := exec.LookPath(n); err == nil {
				return p, "PSResourceGet"
			}
		}
		return "", ""
	}
	return "", ""
}

var doctorCmd = &cobra.Command{
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
		fmt.Fprintf(out, "  Pinned Packages: %d\n\n", pinnedCount)

		var rows []doctorRow
		maxName := 7
		maxStatus := 10
		maxVer := 7

		for _, name := range reg.ManagerNames() {
			mgr, _ := reg.GetManager(name)
			avail := mgr != nil && mgr.IsAvailable(cmd.Context())
			status := "Not Found"
			binPath := "-"
			ver := "-"
			if avail {
				status = "Available"
				p, v := resolveManagerBinaryAndVersion(name)
				if p != "" {
					binPath = p
				}
				if v != "" {
					ver = v
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

func init() {
	rootCmd.AddCommand(doctorCmd)
}
