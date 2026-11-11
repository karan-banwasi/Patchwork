package pipx

import (
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

func TestPipxManager_BasicInfo(t *testing.T) {
	mgr := NewPipxManager()
	if mgr.Name() != "pipx" {
		t.Errorf("expected name pipx, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "pipx-app" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("black"); err != nil {
		t.Errorf("expected black to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-black"); err == nil {
		t.Errorf("expected -black to be invalid ID")
	}
	if !mgr.RequiresNetwork() {
		t.Errorf("expected pipx to require network")
	}
}

func TestPipxManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackage(context.Background(), "black")
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "upgrade black") {
		t.Errorf("expected pipx upgrade args, got %v", capturedArgs)
	}
}

func TestPipxManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleJSON := `{
		"venvs": {
			"black": {
				"metadata": {
					"main_package": {
						"package": "black",
						"package_version": "23.9.1"
					}
				}
			}
		}
	}`

	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleJSON))
			return nil
		},
	}

	client := &http.Client{
		Transport: &mockTransport{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				body := `{"info": {"version": "23.10.0"}}`
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
	if len(updates) != 1 || updates[0].Id != "black" {
		t.Fatalf("expected 1 update for black, got: %+v", updates)
	}
	if updates[0].Version != "23.9.1" || updates[0].AvailableVersion != "23.10.0" {
		t.Errorf("unexpected version change: %+v", updates[0])
	}
}

func TestPipxManager_OfflineMode(t *testing.T) {
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.SetOffline(true)
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(`{"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"23.9.1"}}}}}`))
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

func TestPipxManager_ListFailure(t *testing.T) {
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("fatal pipx error"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fatal pipx error") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
