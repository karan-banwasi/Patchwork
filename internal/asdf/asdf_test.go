package asdf

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

func TestAsdfManager_BasicInfo(t *testing.T) {
	mgr := NewAsdfManager()
	if mgr.Name() != "asdf" {
		t.Errorf("expected name asdf, got %s", mgr.Name())
	}
	if len(mgr.Aliases()) != 0 {
		t.Errorf("expected no aliases, got: %v", mgr.Aliases())
	}
	if err := mgr.ValidateID("nodejs"); err != nil {
		t.Errorf("expected nodejs to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-nodejs"); err == nil {
		t.Errorf("expected -nodejs to be invalid ID")
	}
}

func TestAsdfManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackage(context.Background(), "nodejs")
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "install nodejs latest") {
		t.Errorf("expected asdf install nodejs latest, got %v", capturedArgs)
	}
}

func TestAsdfManager_Refresh(t *testing.T) {
	var capturedArgs []string
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "plugin update --all") {
		t.Errorf("expected plugin update --all, got %v", capturedArgs)
	}
}

func TestAsdfManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleOutput := "nodejs 20.9.0 /home/user/.tool-versions\n"
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "current" {
				stdout.Write([]byte(sampleOutput))
				return nil
			}
			if len(args) > 0 && args[0] == "latest" {
				stdout.Write([]byte("20.10.0\n"))
				return nil
			}
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "nodejs" {
		t.Fatalf("expected 1 update for nodejs, got: %+v", updates)
	}
	if updates[0].Version != "20.9.0" || updates[0].AvailableVersion != "20.10.0" {
		t.Errorf("unexpected version: %+v", updates[0])
	}
}

func TestAsdfManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("asdf not configured"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "asdf not configured") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}

func TestAsdfManager_PartialAndTotalLookupWarnings(t *testing.T) {
	sampleOutput := "nodejs 20.9.0 /home/user/.tool-versions\npython 3.11.0 /home/user/.tool-versions\n"

	// Partial failure -> returns updates + 1 warning
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "current" {
				_, _ = stdout.Write([]byte(sampleOutput))
				return nil
			}
			if len(args) >= 2 && args[0] == "latest" {
				if args[1] == "nodejs" {
					_, _ = stdout.Write([]byte("20.10.0\n"))
					return nil
				}
				return errors.New("network timeout")
			}
			return nil
		},
	}

	updates, warnings, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("expected partial lookup failure to succeed with warning, got err: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "nodejs" {
		t.Errorf("expected nodejs update, got: %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "1 of 2 tools could not be checked") {
		t.Errorf("expected partial failure warning, got: %v", warnings)
	}

	// Total failure -> returns fatal error
	mgrAllFail := NewAsdfManager()
	mgrAllFail.exePath = "asdf"
	mgrAllFail.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "current" {
				_, _ = stdout.Write([]byte(sampleOutput))
				return nil
			}
			return errors.New("network unreachable")
		},
	}

	if _, _, err := mgrAllFail.GetAvailableUpdatesWithWarnings(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to check any asdf tools") {
		t.Errorf("expected fatal error when all lookups fail, got: %v", err)
	}
}
