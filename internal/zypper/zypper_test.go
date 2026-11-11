package zypper

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

func TestZypperManager_BasicInfo(t *testing.T) {
	mgr := NewZypperManager()
	if mgr.Name() != "zypper" {
		t.Errorf("expected name zypper, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 2 || aliases[0] != "opensuse" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("curl"); err != nil {
		t.Errorf("expected curl to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-curl"); err == nil {
		t.Errorf("expected -curl to be invalid ID")
	}
}

func TestZypperManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewZypperManager()
	mgr.exePath = "/usr/bin/zypper"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"curl", "git"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--non-interactive update curl git") {
		t.Errorf("expected update args, got %v", capturedArgs)
	}
}

func TestZypperManager_CleanupAndRefresh(t *testing.T) {
	var commands []string
	mgr := NewZypperManager()
	mgr.exePath = "/usr/bin/zypper"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			commands = append(commands, strings.Join(args, " "))
			return nil
		},
	}

	var buf bytes.Buffer
	if err := mgr.Cleanup(context.Background(), &buf, &buf); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "clean --all") {
		t.Errorf("expected clean --all, got: %v", commands)
	}

	commands = nil
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "refresh") {
		t.Errorf("expected refresh command, got: %v", commands)
	}
}

func TestZypperManager_GetAvailableUpdates_ExitCode100(t *testing.T) {
	mgr := NewZypperManager()
	mgr.exePath = "/usr/bin/zypper"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("v | repo | curl | 8.4.0-1.1 | 8.5.0-1.1 | x86_64\n"))
			// Exit code 100 indicates updates available
			return errors.New("exit status 100")
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on exit code 100: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "curl" {
		t.Errorf("expected 1 update for curl, got: %+v", updates)
	}
}
