package gem

import (
	"bytes"
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

func TestGemManager_BasicInfo(t *testing.T) {
	mgr := NewGemManager()
	if mgr.Name() != "gem" {
		t.Errorf("expected name gem, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 2 || aliases[0] != "rubygems" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("rails"); err != nil {
		t.Errorf("expected rails to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-rails"); err == nil {
		t.Errorf("expected -rails to be invalid ID")
	}
}

func TestGemManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"rails", "bundler"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "update rails bundler") {
		t.Errorf("expected gem update args, got %v", capturedArgs)
	}
}

func TestGemManager_Cleanup(t *testing.T) {
	var capturedArgs []string
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	var buf bytes.Buffer
	if err := mgr.Cleanup(context.Background(), &buf, &buf); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "cleanup") {
		t.Errorf("expected cleanup, got %v", capturedArgs)
	}
}

func TestGemManager_GetAvailableUpdates_Success(t *testing.T) {
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("rails (7.0.8 < 7.1.2)\n"))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "rails" {
		t.Fatalf("expected 1 update for rails, got: %+v", updates)
	}
	if updates[0].Version != "7.0.8" || updates[0].AvailableVersion != "7.1.2" {
		t.Errorf("expected version 7.0.8 -> 7.1.2, got: %+v", updates[0])
	}
}

func TestGemManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("gem source unreachable"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gem source unreachable") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
