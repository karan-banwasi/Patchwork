package apt

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

func TestAptManager_BasicInfo(t *testing.T) {
	mgr := NewAptManager()
	if mgr.Name() != "apt" {
		t.Errorf("expected name apt, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 3 || aliases[0] != "apt-get" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if mgr.FetchTimeout() <= 0 {
		t.Errorf("expected positive timeout, got %v", mgr.FetchTimeout())
	}
	if err := mgr.ValidateID("curl"); err != nil {
		t.Errorf("expected curl to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-curl"); err == nil {
		t.Errorf("expected -curl to be invalid ID")
	}
}

func TestAptManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
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
	if !strings.Contains(joined, "install --only-upgrade -y curl git") {
		t.Errorf("expected install args, got %v", capturedArgs)
	}
}

func TestAptManager_CleanupAndRefresh(t *testing.T) {
	var commands []string
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
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
	if len(commands) < 2 {
		t.Errorf("expected autoremove and clean, got: %v", commands)
	}

	commands = nil
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "update") {
		t.Errorf("expected update command, got: %v", commands)
	}
}

func TestAptManager_GetAvailableUpdates_Failure(t *testing.T) {
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("dpkg locked"))
			return errors.New("exit status 100")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "dpkg locked") {
		t.Errorf("expected error message to contain stderr, got: %v", err)
	}
}
