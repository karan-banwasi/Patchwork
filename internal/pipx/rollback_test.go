package pipx

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

func TestPipxRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &PipxManager{runner: mock, exePath: "pipx"}

	if err := mgr.RollbackPackage(context.Background(), "black", "24.3.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"install", "black==24.3.0", "--force"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "black", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "black", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
