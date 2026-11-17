package mise

import (
	"context"
	"io"
	"reflect"
	"testing"
)

type mockRollbackRunner struct {
	calls [][]string
	err   error
}

func (m *mockRollbackRunner) Run(_ context.Context, _ string, args []string, _ io.Reader, _, _ io.Writer) error {
	m.calls = append(m.calls, append([]string(nil), args...))
	return m.err
}

func TestMiseRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &MiseManager{runner: mock, exePath: "mise"}

	if err := mgr.RollbackPackage(context.Background(), "node", "20.11.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]string{
		{"install", "node@20.11.0"},
		{"use", "-g", "node@20.11.0"},
	}
	if !reflect.DeepEqual(mock.calls, want) {
		t.Errorf("calls = %v, want %v", mock.calls, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "node", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "node", "-g"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
