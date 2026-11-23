package pnpm

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

func TestPnpmRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &PnpmManager{runner: mock, exePath: "pnpm"}

	if err := mgr.RollbackPackage(context.Background(), "turbo", "2.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"add", "-g", "turbo@2.0.1"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "turbo", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "turbo", "-v"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
