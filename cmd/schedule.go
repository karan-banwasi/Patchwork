package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/schedule"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
	"github.com/spf13/cobra"
)

const statusBarCacheMaxAge = 365 * 24 * time.Hour

// Swappable hooks for deterministic unit testing of schedule commands.
var (
	scheduleRunnerFactory = func() pm.Runner { return pm.ExecRunner{} }
	scheduleGOOS          = func() string { return runtime.GOOS }
	scheduleEnableFn      = schedule.Enable
	scheduleDisableFn     = schedule.Disable
	scheduleRegisteredFn  = schedule.IsRegistered
	scheduleNotifyFn      = schedule.SendNotification
)

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled background update checks and desktop notifications",
		Long:  `Configure, inspect, and execute periodic background update checks using your operating system's native user-level scheduler (Task Scheduler on Windows, launchd on macOS, systemd/crontab on Linux).`,
	}

	cmd.AddCommand(
		newScheduleEnableCmd(),
		newScheduleDisableCmd(),
		newScheduleStatusCmd(),
		newScheduleRunCmd(),
	)

	return cmd
}

func newScheduleEnableCmd() *cobra.Command {
	var intervalStr string
	var notify bool

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable periodic background update checks",
		Long:  `Register a user-level scheduled task to run 'pw schedule run' periodically and persist the schedule configuration.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dur, normInterval, err := schedule.ParseInterval(intervalStr)
			if err != nil {
				return err
			}

			runner := scheduleRunnerFactory()
			goos := scheduleGOOS()
			if err := scheduleEnableFn(cmd.Context(), runner, goos, dur, notify); err != nil {
				return err
			}

			st, err := schedule.LoadState()
			if err != nil {
				st = &schedule.State{}
			}
			st.Enabled = true
			st.Interval = normInterval
			st.Notify = notify
			st.UpdatedAt = time.Now().UTC()
			if err := st.Save(); err != nil {
				return fmt.Errorf("failed to save schedule state: %w", err)
			}

			cmd.Printf("Scheduled background checks enabled (interval: %s, notify: %v).\n", normInterval, notify)
			return nil
		},
	}

	cmd.Flags().StringVar(&intervalStr, "interval", schedule.DefaultInterval, "Check interval (hourly, daily, weekly, or Go duration >= 15m)")
	cmd.Flags().BoolVar(&notify, "notify", true, "Send desktop notifications when updates are available")
	return cmd
}

func newScheduleDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable periodic background update checks",
		Long:  `Unregister the user-level scheduled background update check task and mark scheduling as disabled.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := scheduleRunnerFactory()
			goos := scheduleGOOS()
			if err := scheduleDisableFn(cmd.Context(), runner, goos); err != nil {
				return err
			}

			st, err := schedule.LoadState()
			if err != nil {
				st = &schedule.State{}
			}
			st.Enabled = false
			st.UpdatedAt = time.Now().UTC()
			if err := st.Save(); err != nil {
				return fmt.Errorf("failed to save schedule state: %w", err)
			}

			cmd.Println("Scheduled background checks disabled.")
			return nil
		},
	}
}

type scheduleStatusJSON struct {
	SchemaVersion string `json:"schema_version"`
	Enabled       bool   `json:"enabled"`
	Registered    bool   `json:"registered"`
	Interval      string `json:"interval,omitempty"`
	Notify        bool   `json:"notify"`
	LastRun       string `json:"last_run,omitempty"`
	LastCount     int    `json:"last_count"`
	CachedCount   int    `json:"cached_count"`
	CacheValid    bool   `json:"cache_valid"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	StatePath     string `json:"state_path"`
}

func formatStatusBarSummary(count int) string {
	switch count {
	case 0:
		return "pw: up to date"
	case 1:
		return "pw: 1 update"
	default:
		return fmt.Sprintf("pw: %d updates", count)
	}
}

func loadEffectiveCachedUpdates() ([]pm.PackageUpdate, bool) {
	updates, ok := config.LoadUpdateCache(statusBarCacheMaxAge)
	if !ok {
		return nil, false
	}
	if filtered, _, err := filterOutPinned(updates); err == nil {
		return filtered, true
	}
	return updates, true
}

func newScheduleStatusCmd() *cobra.Command {
	var jsonOut bool
	var barOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show scheduled check status or a status-bar summary",
		Long:  `Display the current scheduled background check state, OS scheduler registration status, and cached update summary.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut && barOut {
				return errors.New("cannot use --json and --bar together")
			}

			st, err := schedule.LoadState()
			if err != nil {
				return err
			}

			cachedUpdates, cacheValid := loadEffectiveCachedUpdates()

			if barOut {
				if cacheValid {
					cmd.Println(formatStatusBarSummary(len(cachedUpdates)))
					return nil
				}
				if !st.LastRun.IsZero() {
					cmd.Println(formatStatusBarSummary(st.LastCount))
					return nil
				}
				cmd.Println("pw: not checked")
				return nil
			}

			runner := scheduleRunnerFactory()
			goos := scheduleGOOS()
			registered, regErr := scheduleRegisteredFn(cmd.Context(), runner, goos)
			if regErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not query OS scheduler status: %v\n", regErr)
			}

			if jsonOut {
				payload := scheduleStatusJSON{
					SchemaVersion: upgrade.CurrentSchemaVersion,
					Enabled:       st.Enabled,
					Registered:    registered,
					Interval:      st.Interval,
					Notify:        st.Notify,
					LastCount:     st.LastCount,
					CachedCount:   len(cachedUpdates),
					CacheValid:    cacheValid,
					StatePath:     schedule.StatePath(),
				}
				if !st.LastRun.IsZero() {
					payload.LastRun = st.LastRun.UTC().Format(time.RFC3339)
				}
				if !st.UpdatedAt.IsZero() {
					payload.UpdatedAt = st.UpdatedAt.UTC().Format(time.RFC3339)
				}
				return encodeIndentedJSON(cmd.OutOrStdout(), payload)
			}

			out := cmd.OutOrStdout()
			intervalDisplay := st.Interval
			if intervalDisplay == "" {
				intervalDisplay = schedule.DefaultInterval + " (default)"
			}
			lastRunDisplay := "never"
			if !st.LastRun.IsZero() {
				lastRunDisplay = st.LastRun.Local().Format(time.RFC3339)
			}
			fmt.Fprintf(out, "Enabled:        %v\n", st.Enabled)
			fmt.Fprintf(out, "OS Registered:  %v\n", registered)
			fmt.Fprintf(out, "Interval:       %s\n", intervalDisplay)
			fmt.Fprintf(out, "Notifications:  %v\n", st.Notify)
			fmt.Fprintf(out, "Last Run:       %s\n", lastRunDisplay)
			fmt.Fprintf(out, "Last Count:     %d\n", st.LastCount)
			if cacheValid {
				fmt.Fprintf(out, "Cached Updates: %d\n", len(cachedUpdates))
			}
			fmt.Fprintf(out, "State File:     %s\n", schedule.StatePath())
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output schedule status as JSON")
	cmd.Flags().BoolVar(&barOut, "bar", false, "Output a compact summary suitable for status bars")
	return cmd
}

func formatNotificationMessage(updates []pm.PackageUpdate) (string, string) {
	if len(updates) == 1 {
		u := updates[0]
		name := u.Name
		if name == "" {
			name = u.Id
		}
		return "Patchwork Updates Available", fmt.Sprintf("1 update available: %s (%s -> %s via %s)", name, u.Version, u.AvailableVersion, u.ManagerName)
	}

	const maxListed = 3
	names := make([]string, 0, maxListed)
	for i := 0; i < len(updates) && i < maxListed; i++ {
		n := updates[i].Name
		if n == "" {
			n = updates[i].Id
		}
		names = append(names, n)
	}
	summary := strings.Join(names, ", ")
	if len(updates) > maxListed {
		summary = fmt.Sprintf("%s, and %d more", summary, len(updates)-maxListed)
	}
	return "Patchwork Updates Available", fmt.Sprintf("%d updates available: %s", len(updates), summary)
}

func newScheduleRunCmd() *cobra.Command {
	var (
		notify  bool
		quiet   bool
		offline bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a scheduled background check now",
		Long:  `Fetch available updates across active package managers, refresh the local update cache, update schedule state, and optionally send a desktop notification.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			statePath := schedule.StatePath()
			_, statErr := os.Stat(statePath)
			stateExists := statErr == nil

			st, err := schedule.LoadState()
			if err != nil {
				fmt.Fprintf(errOut, "Warning: failed to load schedule state: %v\n", err)
				st = &schedule.State{Notify: true}
			}
			if !stateExists {
				st.Notify = true
			}

			shouldNotify := notify
			if stateExists && !cmd.Flags().Changed("notify") {
				shouldNotify = st.Notify
			}

			registry, cfg := getDefaultRegistryAndConfig()
			configureRegistryIO(registry, cmd.InOrStdin(), out, errOut)
			hydrateCommonDefaults(cmd, cfg, &timeout, &registry.SkipRefresh, nil, nil)

			if offline {
				registry.SetOffline(true)
			}
			if timeout > 0 {
				registry.FetchTimeout = timeout
			}

			updates, warnings, err := registry.FetchUpdates(cmd.Context())
			for _, w := range warnings {
				fmt.Fprintf(errOut, "Warning: %v\n", w)
			}
			if err != nil {
				return err
			}

			if cacheErr := config.SaveUpdateCache(updates); cacheErr != nil {
				fmt.Fprintf(errOut, "Warning: failed to save update cache: %v\n", cacheErr)
			}

			effective := updates
			if filtered, _, pinErr := filterOutPinned(updates); pinErr == nil {
				effective = filtered
			}

			now := time.Now().UTC()
			st.LastRun = now
			st.LastCount = len(effective)
			st.UpdatedAt = now
			if saveErr := st.Save(); saveErr != nil {
				return fmt.Errorf("failed to save schedule state: %w", saveErr)
			}

			if len(effective) > 0 && shouldNotify {
				title, msg := formatNotificationMessage(effective)
				runner := scheduleRunnerFactory()
				goos := scheduleGOOS()
				if notifyErr := scheduleNotifyFn(cmd.Context(), runner, goos, title, msg); notifyErr != nil {
					// If command context was canceled, respect cancellation; otherwise warn non-fatally.
					if cmd.Context().Err() != nil {
						return cmd.Context().Err()
					}
					fmt.Fprintf(errOut, "Warning: failed to send desktop notification: %v\n", notifyErr)
				}
			}

			if !quiet {
				if len(effective) == 0 {
					fmt.Fprintln(out, "No updates available. Everything is up to date!")
				} else {
					fmt.Fprintf(out, "Found %d available update(s).\n", len(effective))
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&notify, "notify", true, "Send a desktop notification when updates are found")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress normal console output")
	cmd.Flags().BoolVar(&offline, "offline", false, "Disable network access and check using only local caches")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Per-manager timeout when querying updates (0 uses manager defaults)")
	return cmd
}
