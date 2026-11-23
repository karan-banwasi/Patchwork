package pnpm

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

func TestPnpmManager_BasicInfo(t *testing.T) {
	mgr := NewPnpmManager()
	if mgr.Name() != "pnpm" {
		t.Errorf("expected name pnpm, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "pnpm-global" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("typescript"); err != nil {
		t.Errorf("expected typescript to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("@scope/pkg"); err != nil {
		t.Errorf("expected @scope/pkg to be valid ID, got: %v", err)
	}
}

func TestPnpmManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewPnpmManager()
	mgr.exePath = "pnpm"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"typescript", "prettier"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "update -g typescript prettier") {
		t.Errorf("expected update args, got %v", capturedArgs)
	}
}

func TestPnpmManager_Cleanup(t *testing.T) {
	var capturedArgs []string
	mgr := NewPnpmManager()
	mgr.exePath = "pnpm"
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
	if !strings.Contains(joined, "store prune") {
		t.Errorf("expected store prune, got %v", capturedArgs)
	}
}

func TestPnpmManager_GetAvailableUpdates_ExitCode1(t *testing.T) {
	sampleJSON := `{"prettier": {"current": "3.0.3", "latest": "3.1.0", "wanted": "3.0.3"}}`
	mgr := NewPnpmManager()
	mgr.exePath = "pnpm"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleJSON))
			// Non-zero exit code when outdated packages are present
			return errors.New("exit status 1")
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on exit code 1 with stdout: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "prettier" {
		t.Fatalf("expected 1 update for prettier, got: %+v", updates)
	}
	if updates[0].Version != "3.0.3" || updates[0].AvailableVersion != "3.1.0" {
		t.Errorf("unexpected version: %+v", updates[0])
	}
}
