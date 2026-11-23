package vscode

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

func TestVSCodeRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &VSCodeManager{runner: mock, exePath: "code"}

	if err := mgr.RollbackPackage(context.Background(), "golang.go", "0.41.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--install-extension", "golang.go@0.41.0", "--force"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "golang.go", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "golang.go", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
