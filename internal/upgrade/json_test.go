package upgrade

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestSummary_RenderJSON(t *testing.T) {
	pkgErr := errors.New("install failure")
	summary := Summary{
		Total:     3,
		Succeeded: 1,
		Failed:    1,
		Skipped:   1,
		Results: []Result{
			{
				Package: pm.PackageUpdate{
					Name:             "Git",
					Id:               "Git.Git",
					Version:          "2.40.0",
					AvailableVersion: "2.41.0",
					ManagerName:      "winget",
				},
				Status:   StatusSucceeded,
				Duration: 500 * time.Millisecond,
			},
			{
				Package: pm.PackageUpdate{
					Name:             "Node",
					Id:               "nodejs",
					Version:          "18.0.0",
					AvailableVersion: "20.0.0",
					ManagerName:      "choco",
				},
				Status:   StatusFailed,
				Duration: 200 * time.Millisecond,
				Error:    pkgErr,
			},
			{
				Package: pm.PackageUpdate{
					Name:             "Python",
					Id:               "python",
					Version:          "3.10",
					AvailableVersion: "3.11",
					ManagerName:      "scoop",
				},
				Status: StatusSkipped,
			},
		},
	}

	var buf bytes.Buffer
	if err := summary.RenderJSON(&buf, false); err != nil {
		t.Fatalf("unexpected error rendering json: %v", err)
	}

	var parsed JSONSummary
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version '1.0.0', got %q", parsed.SchemaVersion)
	}
	if parsed.Total != 3 || parsed.Succeeded != 1 || parsed.Failed != 1 || parsed.Skipped != 1 {
		t.Errorf("summary counts mismatch: %+v", parsed)
	}
	if len(parsed.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(parsed.Results))
	}
	if parsed.Results[0].Status != string(StatusSucceeded) || parsed.Results[0].Error != nil {
		t.Errorf("result 0 mismatch: %+v", parsed.Results[0])
	}
	if parsed.Results[1].Status != string(StatusFailed) || parsed.Results[1].Error == nil || !strings.Contains(*parsed.Results[1].Error, "install failure") {
		t.Errorf("result 1 error message mismatch: %+v", parsed.Results[1])
	}
	if parsed.Results[2].Status != string(StatusSkipped) {
		t.Errorf("result 2 status mismatch: %+v", parsed.Results[2])
	}
}
