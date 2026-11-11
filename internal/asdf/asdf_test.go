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
