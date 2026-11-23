package uv

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

func TestUvRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &UvManager{runner: mock}

	if err := mgr.RollbackPackage(context.Background(), "ruff", "0.4.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"tool", "install", "ruff==0.4.1", "--force"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "ruff", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "ruff", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
