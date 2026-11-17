package npm

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type mockRunner struct {
	runFunc func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

func (m *mockRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, name, args, stdin, stdout, stderr)
	}
	return nil
}

func TestParseNpmOutdatedJSON(t *testing.T) {
	sample := `{
  "typescript": {
    "current": "5.4.2",
    "wanted": "5.5.4",
    "latest": "5.5.4",
    "dependent": "global"
  },
  "downgrade-pkg": {
    "current": "2.0.0",
    "wanted": "1.9.0",
    "latest": "1.9.0",
    "dependent": "global"
  },
  "alpha-pkg": {
    "current": "1.0.0",
    "wanted": "1.1.0",
    "latest": "1.1.0",
    "dependent": "global"
  },
  "@angular/cli": {
    "current": "17.0.0",
    "wanted": "17.3.0",
    "latest": "18.1.0",
    "dependent": "global"
  }
}`
	updates, err := parseNpmOutdatedJSON(sample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 3 {
		t.Fatalf("expected 3 updates (downgrade excluded), got %d: %+v", len(updates), updates)
	}
	if updates[0].Id != "@angular/cli" || updates[1].Id != "alpha-pkg" || updates[2].Id != "typescript" {
		t.Errorf("expected sorted order (@angular/cli, alpha-pkg, typescript), got: %+v", updates)
	}

	if _, err := parseNpmOutdatedJSON(`{"error":{"summary":"network failure"}}`); err == nil {
		t.Errorf("expected error for npm error JSON payload")
	}
	if _, err := parseNpmOutdatedJSON(`not-json`); err == nil {
		t.Errorf("expected error for invalid JSON payload")
	}
}

func TestNpmManager_GetAvailableUpdates_ErrorSurfaced(t *testing.T) {
	mgr := NewNpmManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			_, _ = stderr.Write([]byte("npm ERR! EACCES"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil || !strings.Contains(err.Error(), "npm outdated failed") {
		t.Fatalf("expected npm outdated failure error, got: %v", err)
	}
}
