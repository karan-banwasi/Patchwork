package mas

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

func TestMasManager_BasicInfo(t *testing.T) {
	mgr := NewMasManager()
	if mgr.Name() != "mas" {
		t.Errorf("expected name mas, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 2 || aliases[0] != "macappstore" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("497799835"); err != nil {
		t.Errorf("expected 497799835 to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("xcode"); err == nil {
		t.Errorf("expected non-numeric ID to be invalid")
	}
}

func TestMasManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewMasManager()
	mgr.exePath = "/usr/local/bin/mas"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"497799835", "1295203466"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "upgrade 497799835 1295203466") {
		t.Errorf("expected mas upgrade args, got %v", capturedArgs)
	}
}

func TestMasManager_GetAvailableUpdates_Success(t *testing.T) {
	mgr := NewMasManager()
	mgr.exePath = "/usr/local/bin/mas"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("497799835 Xcode (15.0 -> 15.1)\n"))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "497799835" {
		t.Fatalf("expected 1 update, got: %+v", updates)
	}
	if updates[0].Version != "15.0" || updates[0].AvailableVersion != "15.1" {
		t.Errorf("expected version 15.0 -> 15.1, got: %+v", updates[0])
	}
}

func TestMasManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewMasManager()
	mgr.exePath = "/usr/local/bin/mas"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("Not signed in"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Not signed in") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
