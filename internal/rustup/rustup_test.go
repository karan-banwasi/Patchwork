package rustup

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

func TestRustupManager_BasicInfo(t *testing.T) {
	mgr := NewRustupManager()
	if mgr.Name() != "rustup" {
		t.Errorf("expected name rustup, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "rust-toolchain" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("stable-x86_64-unknown-linux-gnu"); err != nil {
		t.Errorf("expected valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-stable"); err == nil {
		t.Errorf("expected -stable to be invalid ID")
	}
}

func TestRustupManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"stable", "nightly"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "update stable nightly") {
		t.Errorf("expected rustup update args, got %v", capturedArgs)
	}
}

func TestRustupManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleOutput := `stable-x86_64-unknown-linux-gnu - Update available : 1.74.0 -> 1.74.1
rustup - Up to date : 1.26.0
`
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleOutput))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "stable-x86_64-unknown-linux-gnu" {
		t.Fatalf("expected 1 update, got: %+v", updates)
	}
	if updates[0].Version != "1.74.0" || updates[0].AvailableVersion != "1.74.1" {
		t.Errorf("unexpected version: %+v", updates[0])
	}
}

func TestRustupManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("error checking toolchains"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "error checking toolchains") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
