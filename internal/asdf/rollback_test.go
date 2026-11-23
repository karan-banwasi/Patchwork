package asdf

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

func TestAsdfRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &AsdfManager{runner: mock, exePath: "asdf"}

	if err := mgr.RollbackPackage(context.Background(), "nodejs", "20.11.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]string{
		{"install", "nodejs", "20.11.0"},
		{"set", "-u", "nodejs", "20.11.0"},
	}
	if !reflect.DeepEqual(mock.calls, want) {
		t.Errorf("calls = %v, want %v", mock.calls, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "nodejs", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "nodejs", "-u"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
