package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karan-banwasi/patchwork/internal/history"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
	"github.com/spf13/cobra"
)

type historyOptions struct {
	limit   int
	manager string
	json    bool
	clear   bool
}

type historyJSONOutput struct {
	SchemaVersion string          `json:"schema_version"`
	Count         int             `json:"count"`
	Entries       []history.Entry `json:"entries"`
}

type historyClearJSONOutput struct {
	SchemaVersion string `json:"schema_version"`
	Cleared       bool   `json:"cleared"`
}

func newHistoryCmd() *cobra.Command {
	opts := historyOptions{
		limit: 20,
	}

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show or clear upgrade history",
		Long:  `Display past package upgrade runs recorded by Patchwork, or clear the stored history.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if opts.clear {
				if err := history.Clear(); err != nil {
					return err
				}
				if opts.json {
					return encodeIndentedJSON(out, historyClearJSONOutput{
						SchemaVersion: upgrade.CurrentSchemaVersion,
						Cleared:       true,
					})
				}
				fmt.Fprintln(out, "Upgrade history cleared.")
				return nil
			}

			var mgrFilter string
			if opts.manager != "" {
				registry := getDefaultRegistry()
				if mgr, exists := registry.GetManager(opts.manager); exists {
					mgrFilter = mgr.Name()
				} else {
					return fmt.Errorf("unknown package manager %q (supported: %s)", opts.manager, strings.Join(registry.ManagerNames(), ", "))
				}
			}

			entries, err := history.LoadAll()
			if err != nil {
				return err
			}

			filtered := filterAndOrderHistoryEntries(entries, mgrFilter, opts.limit)

			if opts.json {
				return encodeIndentedJSON(out, historyJSONOutput{
					SchemaVersion: upgrade.CurrentSchemaVersion,
					Count:         len(filtered),
					Entries:       filtered,
				})
			}

			renderHistoryEntries(out, filtered)
			return nil
		},
	}

	cmd.Flags().IntVarP(&opts.limit, "limit", "n", 20, "Maximum number of history runs to show (0 for all)")
	cmd.Flags().StringVarP(&opts.manager, "manager", "m", "", "Filter history by package manager")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output history as JSON")
	cmd.Flags().BoolVar(&opts.clear, "clear", false, "Clear all recorded upgrade history")
	_ = cmd.RegisterFlagCompletionFunc("manager", completeManagerNames)

	return cmd
}

func filterAndOrderHistoryEntries(entries []history.Entry, mgrFilter string, limit int) []history.Entry {
	out := make([]history.Entry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if mgrFilter != "" {
			var matchedItems []history.Item
			var succeeded, failed, skipped int
			for _, item := range e.Items {
				if !strings.EqualFold(item.Manager, mgrFilter) {
					continue
				}
				matchedItems = append(matchedItems, item)
				switch {
				case strings.EqualFold(item.Status, string(upgrade.StatusSucceeded)):
					succeeded++
				case strings.EqualFold(item.Status, string(upgrade.StatusFailed)):
					failed++
				default:
					skipped++
				}
			}
			if len(matchedItems) == 0 {
				continue
			}
			e.Items = matchedItems
			e.Succeeded = succeeded
			e.Failed = failed
			e.Skipped = skipped
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func renderHistoryEntries(out io.Writer, entries []history.Entry) {
	if len(entries) == 0 {
		fmt.Fprintln(out, "No upgrade history found.")
		return
	}

	for idx, entry := range entries {
		if idx > 0 {
			fmt.Fprintln(out)
		}
		ts := entry.Timestamp.Local().Format("2006-01-02 15:04:05")
		fmt.Fprintf(out, "Run %s (%s)  [Succeeded: %d | Failed: %d | Skipped: %d]\n",
			entry.RunID, ts, entry.Succeeded, entry.Failed, entry.Skipped)
		if len(entry.Items) == 0 {
			continue
		}

		maxID := len("Package ID")
		maxMgr := len("Manager")
		maxVer := len("Version")
		maxStatus := len("Status")

		verStrings := make([]string, len(entry.Items))
		for i, item := range entry.Items {
			ver := formatVersionTransition(item.FromVersion, item.ToVersion)
			verStrings[i] = ver
			if n := utf8.RuneCountInString(item.ID); n > maxID {
				maxID = n
			}
			if n := utf8.RuneCountInString(item.Manager); n > maxMgr {
				maxMgr = n
			}
			if n := utf8.RuneCountInString(ver); n > maxVer {
				maxVer = n
			}
			if n := utf8.RuneCountInString(item.Status); n > maxStatus {
				maxStatus = n
			}
		}

		rowFmt := fmt.Sprintf("  %%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n", maxID, maxMgr, maxVer, maxStatus)
		fmt.Fprintf(out, rowFmt, "Package ID", "Manager", "Version", "Status", "Duration")
		fmt.Fprintf(out, rowFmt, strings.Repeat("-", maxID), strings.Repeat("-", maxMgr), strings.Repeat("-", maxVer), strings.Repeat("-", maxStatus), "--------")
		for i, item := range entry.Items {
			durStr := "-"
			if item.DurationMs > 0 {
				durStr = (time.Duration(item.DurationMs) * time.Millisecond).Round(100 * time.Millisecond).String()
			}
			fmt.Fprintf(out, rowFmt, item.ID, item.Manager, verStrings[i], item.Status, durStr)
		}
	}
}

func formatVersionTransition(from, to string) string {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	switch {
	case from != "" && to != "":
		return from + " -> " + to
	case from != "":
		return from + " -> ?"
	case to != "":
		return "? -> " + to
	default:
		return "-"
	}
}

func encodeIndentedJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
