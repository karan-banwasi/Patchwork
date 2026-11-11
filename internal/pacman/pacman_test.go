package pacman

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

func TestPacmanManager_BasicInfo(t *testing.T) {
	mgr := NewPacmanManager()
	if mgr.Name() != "pacman" {
		t.Errorf("expected name pacman, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "arch" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("curl"); err != nil {
		t.Errorf("expected curl to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-curl"); err == nil {
		t.Errorf("expected -curl to be invalid ID")
	}
}

func TestPacmanManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
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
	if !strings.Contains(joined, "-S --noconfirm curl git") {
		t.Errorf("expected pacman -S args, got %v", capturedArgs)
	}
}

func TestPacmanManager_CleanupAndRefresh(t *testing.T) {
	var commands []string
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
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
	if len(commands) != 1 || !strings.Contains(commands[0], "-Sc --noconfirm") {
		t.Errorf("expected -Sc --noconfirm, got: %v", commands)
	}

	commands = nil
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "-Sy") {
		t.Errorf("expected -Sy command, got: %v", commands)
	}
}

func TestPacmanManager_GetAvailableUpdates_ExitCode2(t *testing.T) {
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			// checkupdates exit code 2 means system up to date
			return errors.New("exit status 2")
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on up-to-date exit: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates, got: %+v", updates)
	}
}
