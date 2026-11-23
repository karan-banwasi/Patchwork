package mise

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

func TestMiseManager_BasicInfo(t *testing.T) {
	mgr := NewMiseManager()
	if mgr.Name() != "mise" {
		t.Errorf("expected name mise, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "rtx" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("node"); err != nil {
		t.Errorf("expected node to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-node"); err == nil {
		t.Errorf("expected -node to be invalid ID")
	}
}

func TestMiseManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewMiseManager()
	mgr.exePath = "mise"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"node", "python"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "upgrade node python") {
		t.Errorf("expected mise upgrade args, got %v", capturedArgs)
	}
}

func TestMiseManager_Cleanup(t *testing.T) {
	var capturedArgs []string
	mgr := NewMiseManager()
	mgr.exePath = "mise"
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
	if !strings.Contains(joined, "prune") {
		t.Errorf("expected prune, got %v", capturedArgs)
	}
}

func TestMiseManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleJSON := `[{"name": "node", "current": "20.9.0", "latest": "20.10.0"}]`
	mgr := NewMiseManager()
	mgr.exePath = "mise"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleJSON))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "node" {
		t.Fatalf("expected 1 update for node, got: %+v", updates)
	}
	if updates[0].Version != "20.9.0" || updates[0].AvailableVersion != "20.10.0" {
		t.Errorf("unexpected version: %+v", updates[0])
	}
}

func TestMiseManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewMiseManager()
	mgr.exePath = "mise"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("config parse failure"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "config parse failure") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
