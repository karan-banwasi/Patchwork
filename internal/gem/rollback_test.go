package gem

import (
	"context"
	"io"
	"reflect"
	"testing"
)

type mockRollbackRunner struct {
	exe  string
	args []string
	err  error
}

func (m *mockRollbackRunner) Run(_ context.Context, name string, args []string, _ io.Reader, _, _ io.Writer) error {
	m.exe = name
	m.args = append([]string(nil), args...)
	return m.err
}

func TestGemRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &GemManager{runner: mock, exePath: "gem"}

	if err := mgr.RollbackPackage(context.Background(), "rubocop", "1.62.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"install", "rubocop", "-v", "1.62.0"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "rubocop", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "rubocop", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
