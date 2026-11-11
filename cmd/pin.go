package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/spf13/cobra"
)

// filterOutPinned removes packages that are pinned in the user's configuration.
// It fails closed: if the configuration file exists but cannot be parsed, it returns an error.
func filterOutPinned(updates []pm.PackageUpdate) ([]pm.PackageUpdate, int, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to load configuration: %w", err)
	}
	if len(cfg.PinnedPackages) == 0 && len(cfg.Pins) == 0 {
		return updates, 0, nil
	}
	var filtered []pm.PackageUpdate
	var skipped int
	for _, u := range updates {
		_, violated := cfg.MatchPin(u.ManagerName, u.Id, u.Name, u.AvailableVersion)
		if violated {
			skipped++
			continue
		}
		filtered = append(filtered, u)
	}
	return filtered, skipped, nil
}

func parseExpiry(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
		return &t, nil
	}
	if strings.HasSuffix(s, "d") || strings.HasSuffix(s, "D") {
		daysStr := s[:len(s)-1]
		if days, err := strconv.Atoi(daysStr); err == nil {
			t := time.Now().AddDate(0, 0, days).UTC()
			return &t, nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		t := time.Now().Add(d).UTC()
		return &t, nil
	}
	return nil, fmt.Errorf("invalid expiry format %q (expected RFC3339, YYYY-MM-DD, or duration like 30d, 24h)", s)
}

func newPinCmd() *cobra.Command {
	var versionFlag string
	var reasonFlag string
	var expiresFlag string

	cmd := &cobra.Command{
		Use:               "pin <package-id> [package-id...]",
		Short:             "Pin one or more packages to ignore them during check and upgrade",
		Long:              `Add one or more package IDs (or manager-scoped entries like winget:Git.Git) to the Patchwork ignore list so they are skipped during 'pw check' and 'pw upgrade'.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeAvailablePackageIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			exp, err := parseExpiry(expiresFlag)
			if err != nil {
				return err
			}

			anyChanged := false
			for _, id := range args {
				entry := config.PinEntry{
					ID:        id,
					Version:   versionFlag,
					Reason:    reasonFlag,
					ExpiresAt: exp,
				}
				if versionFlag == "" && reasonFlag == "" && exp == nil {
					if !cfg.Pin(id) {
						cmd.Printf("Package %q is already pinned.\n", id)
						continue
					}
					anyChanged = true
					cmd.Printf("Pinned package %q (will be skipped during check and upgrade).\n", id)
				} else {
					cfg.AddPinEntry(entry)
					anyChanged = true
					var details []string
					if versionFlag != "" {
						details = append(details, fmt.Sprintf("version: %s", versionFlag))
					}
					if reasonFlag != "" {
						details = append(details, fmt.Sprintf("reason: %s", reasonFlag))
					}
					if exp != nil {
						details = append(details, fmt.Sprintf("expires: %s", exp.Format("2006-01-02")))
					}
					cmd.Printf("Pinned package %q (%s).\n", id, strings.Join(details, ", "))
				}
			}
			if anyChanged {
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&versionFlag, "version", "", "Version constraint for pin (e.g. '<23', '~22.0.0')")
	cmd.Flags().StringVar(&reasonFlag, "reason", "", "Reason for pinning this package")
	cmd.Flags().StringVar(&expiresFlag, "expires", "", "Expiration date or duration for pin (e.g. '2026-12-31', '30d')")

	return cmd
}

func newUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <package-id> [package-id...]",
		Short: "Unpin one or more previously pinned packages",
		Long:  `Remove one or more package IDs (or manager:id entries) from the Patchwork ignore list so their updates are shown again.`,
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cfg, err := config.Load()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			already := make(map[string]bool, len(args))
			for _, a := range args {
				already[strings.ToLower(a)] = true
			}
			var out []string
			for _, p := range cfg.PinnedPackages {
				if !already[strings.ToLower(p)] {
					out = append(out, p)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			var missing []string
			anyChanged := false
			for _, id := range args {
				if !cfg.Unpin(id) {
					missing = append(missing, id)
					continue
				}
				anyChanged = true
				cmd.Printf("Unpinned package %q.\n", id)
			}
			if anyChanged {
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("package %q is not currently pinned", strings.Join(missing, ", "))
			}
			return nil
		},
	}
}

func newPinnedCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "pinned",
		Short: "List all pinned (ignored) packages",
		Long:  `Display all packages currently pinned in the Patchwork configuration.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if jsonOutput {
				type pinnedPkg struct {
					Raw       string     `json:"raw"`
					ID        string     `json:"id"`
					Manager   string     `json:"manager,omitempty"`
					Version   string     `json:"version,omitempty"`
					Reason    string     `json:"reason,omitempty"`
					ExpiresAt *time.Time `json:"expires_at,omitempty"`
					Expired   bool       `json:"expired"`
				}
				type jsonPinnedOutput struct {
					SchemaVersion string      `json:"schema_version"`
					Count         int         `json:"count"`
					Packages      []pinnedPkg `json:"pinned_packages"`
				}
				out := jsonPinnedOutput{
					SchemaVersion: "1.0.0",
					Count:         len(cfg.Pins),
					Packages:      make([]pinnedPkg, 0, len(cfg.Pins)),
				}
				for _, p := range cfg.Pins {
					rawID := p.ID
					if p.Manager != "" {
						rawID = p.Manager + ":" + p.ID
					}
					item := pinnedPkg{
						Raw:       rawID,
						ID:        p.ID,
						Manager:   p.Manager,
						Version:   p.Version,
						Reason:    p.Reason,
						ExpiresAt: p.ExpiresAt,
						Expired:   p.IsExpired(),
					}
					out.Packages = append(out.Packages, item)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			if len(cfg.Pins) == 0 {
				cmd.Println("No packages are currently pinned.")
				return nil
			}
			cmd.Printf("Pinned packages (%d):\n", len(cfg.Pins))
			for _, p := range cfg.Pins {
				mgrStr := "all managers"
				if p.Manager != "" {
					mgrStr = "manager: " + p.Manager
				}
				line := fmt.Sprintf("  - %s (%s)", p.ID, mgrStr)
				if p.Version != "" {
					line += fmt.Sprintf(" [constraint: %s]", p.Version)
				}
				if p.Reason != "" {
					line += fmt.Sprintf(" [reason: %s]", p.Reason)
				}
				if p.ExpiresAt != nil {
					line += fmt.Sprintf(" [expires: %s]", p.ExpiresAt.Format("2006-01-02"))
					if p.IsExpired() {
						line += " [EXPIRED]"
					}
				}
				cmd.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output pinned packages in JSON format")
	return cmd
}
