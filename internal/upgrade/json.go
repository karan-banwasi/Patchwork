package upgrade

import (
	"encoding/json"
	"io"
	"time"
)

// CurrentSchemaVersion is the semver identifier for Patchwork JSON outputs.
const CurrentSchemaVersion = "1.0.0"

// JSONPackage represents package identity in JSON outputs.
type JSONPackage struct {
	Name             string `json:"name"`
	ID               string `json:"id"`
	Version          string `json:"version"`
	AvailableVersion string `json:"available_version"`
	Manager          string `json:"manager"`
}

// JSONResult represents a single package upgrade result in JSON outputs.
type JSONResult struct {
	Package    JSONPackage `json:"package"`
	Status     string      `json:"status"`
	DurationMs int64       `json:"duration_ms"`
	Duration   string      `json:"duration"`
	Error      *string     `json:"error"`
}

// JSONSummary is the top-level machine-readable structure emitted by `pw upgrade --json`.
type JSONSummary struct {
	SchemaVersion string       `json:"schema_version"`
	DryRun        bool         `json:"dry_run"`
	Total         int          `json:"total"`
	Succeeded     int          `json:"succeeded"`
	Failed        int          `json:"failed"`
	Skipped       int          `json:"skipped"`
	Results       []JSONResult `json:"results"`
}

// ToJSONSummary converts a Summary into a JSONSummary.
func (s Summary) ToJSONSummary(dryRun bool) JSONSummary {
	results := make([]JSONResult, 0, len(s.Results))
	for _, r := range s.Results {
		var errStr *string
		if r.Error != nil {
			msg := r.Error.Error()
			errStr = &msg
		}
		durStr := r.Duration.Round(100 * time.Millisecond).String()
		if r.Duration == 0 {
			durStr = "-"
		}
		results = append(results, JSONResult{
			Package: JSONPackage{
				Name:             r.Package.Name,
				ID:               r.Package.Id,
				Version:          r.Package.Version,
				AvailableVersion: r.Package.AvailableVersion,
				Manager:          r.Package.ManagerName,
			},
			Status:     string(r.Status),
			DurationMs: r.Duration.Milliseconds(),
			Duration:   durStr,
			Error:      errStr,
		})
	}
	return JSONSummary{
		SchemaVersion: CurrentSchemaVersion,
		DryRun:        dryRun,
		Total:         s.Total,
		Succeeded:     s.Succeeded,
		Failed:        s.Failed,
		Skipped:       s.Skipped,
		Results:       results,
	}
}

// RenderJSON serializes the summary as formatted JSON to out.
func (s Summary) RenderJSON(out io.Writer, dryRun bool) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(s.ToJSONSummary(dryRun))
}
