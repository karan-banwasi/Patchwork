package dnf

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

func TestDnfManager_BasicInfo(t *testing.T) {
	mgr := NewDnfManager()
	if mgr.Name() != "dnf" {
		t.Errorf("expected name dnf, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 3 || aliases[0] != "yum" {
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

func TestDnfManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
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
	if !strings.Contains(joined, "upgrade -y curl git") {
		t.Errorf("expected upgrade args, got %v", capturedArgs)
	}
}

func TestDnfManager_CleanupAndRefresh(t *testing.T) {
	var commands []string
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
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
		t.Errorf("expected clean and autoremove, got: %v", commands)
	}

	commands = nil
	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "makecache") {
		t.Errorf("expected makecache command, got: %v", commands)
	}
}

func TestDnfManager_GetAvailableUpdates_ExitCode100(t *testing.T) {
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("curl.x86_64 7.82.0-15.fc38 updates\n"))
			// Exit code 100 indicates updates found
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

func TestDnfManager_InstalledMapFiltering(t *testing.T) {
	input := `curl.x86_64 7.82.0-15.fc38 updates
git.x86_64 2.40.1-1.fc38 updates
`
	installed := map[string]string{
		"curl": "7.82.0-14.fc38",
		"git":  "2.40.1-2.fc38", // newer installed -> downgrade filtered!
	}
	updates, err := parseDnfCheckUpdate(input, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "curl" {
		t.Errorf("expected only curl to be an upgrade, got: %+v", updates)
	}
	if updates[0].Version != "7.82.0-14.fc38" {
		t.Errorf("expected installed version 7.82.0-14.fc38, got %s", updates[0].Version)
	}
}
