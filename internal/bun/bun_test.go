package bun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
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

type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestBunManager_BasicInfo(t *testing.T) {
	mgr := NewBunManager()
	if mgr.Name() != "bun" {
		t.Errorf("expected name bun, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "bun-global" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("prettier"); err != nil {
		t.Errorf("expected prettier to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("@scope/tool"); err != nil {
		t.Errorf("expected @scope/tool to be valid ID, got: %v", err)
	}
	if !mgr.RequiresNetwork() {
		t.Errorf("expected bun to require network")
	}
}

func TestBunManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"prettier", "typescript"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "add -g prettier@latest typescript@latest") {
		t.Errorf("expected add -g with @latest, got: %v", capturedArgs)
	}
}

func TestBunManager_Cleanup(t *testing.T) {
	var capturedArgs []string
	mgr := NewBunManager()
	mgr.exePath = "bun"
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
	if !strings.Contains(joined, "pm cache rm") {
		t.Errorf("expected pm cache rm, got: %v", capturedArgs)
	}
}

func TestBunManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleOutput := `/home/user/.bun/install/global
└── prettier@3.0.3
`
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleOutput))
			return nil
		},
	}

	client := &http.Client{
		Transport: &mockTransport{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				body := `{"version": "3.1.0"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}
	mgr.SetHTTPClient(client)

	updates, warnings, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", warnings)
	}
	if len(updates) != 1 || updates[0].Id != "prettier" {
		t.Fatalf("expected 1 update for prettier, got: %+v", updates)
	}
	if updates[0].Version != "3.0.3" || updates[0].AvailableVersion != "3.1.0" {
		t.Errorf("unexpected version change: %+v", updates[0])
	}
}

func TestBunManager_OfflineMode(t *testing.T) {
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.SetOffline(true)
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("/home/user/.bun/install/global\n└── prettier@3.0.3\n"))
			return nil
		},
	}

	updates, warnings, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates in offline mode, got %d", len(updates))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "offline mode") {
		t.Errorf("expected offline warning, got %v", warnings)
	}
}

func TestBunManager_ListFailure(t *testing.T) {
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("failed to read global store"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read global store") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
