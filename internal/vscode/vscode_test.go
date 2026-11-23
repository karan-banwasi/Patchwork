package vscode

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

func TestVSCodeManager_BasicInfo(t *testing.T) {
	mgr := NewVSCodeManager()
	if mgr.Name() != "vscode" {
		t.Errorf("expected name vscode, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 2 || aliases[0] != "code" || aliases[1] != "vs-code" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("golang.go"); err != nil {
		t.Errorf("expected golang.go to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("invalid_id"); err == nil {
		t.Errorf("expected invalid_id to be invalid ID")
	}
	if !mgr.RequiresNetwork() {
		t.Errorf("expected vscode to require network")
	}
}

func TestVSCodeManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"golang.go", "esbenp.prettier-vscode"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--install-extension golang.go --force --install-extension esbenp.prettier-vscode --force") {
		t.Errorf("expected vscode install args, got %v", capturedArgs)
	}
}

func TestVSCodeManager_GetAvailableUpdates_Success(t *testing.T) {
	sampleOutput := "golang.go@0.40.0\nms-python.python@2023.20.0\n"

	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte(sampleOutput))
			return nil
		},
	}

	client := &http.Client{
		Transport: &mockTransport{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				reqBody, _ := io.ReadAll(req.Body)
				var versionStr string
				if strings.Contains(string(reqBody), "golang.go") {
					versionStr = "0.41.0"
				} else {
					versionStr = "2023.20.0"
				}
				body := `{"results":[{"extensions":[{"versions":[{"version":"` + versionStr + `"}]}]}]}`
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
	if len(updates) != 1 || updates[0].Id != "golang.go" {
		t.Fatalf("expected 1 update for golang.go, got: %+v", updates)
	}
	if updates[0].Version != "0.40.0" || updates[0].AvailableVersion != "0.41.0" {
		t.Errorf("unexpected version change: %+v", updates[0])
	}
}

func TestVSCodeManager_OfflineMode(t *testing.T) {
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.SetOffline(true)
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stdout.Write([]byte("golang.go@0.40.0\n"))
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

func TestVSCodeManager_ListFailure(t *testing.T) {
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("fatal vscode error"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fatal vscode error") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
