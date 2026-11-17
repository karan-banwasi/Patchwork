package choco

import (
	"context"
	"reflect"
	"testing"
)

func TestChocoRollbackPackage(t *testing.T) {
	mock := &mockChocoRunner{}
	mgr := &ChocoManager{runner: mock}

	if err := mgr.RollbackPackage(context.Background(), "git.install", "2.44.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"install", "git.install", "--version", "2.44.0", "--allow-downgrade", "-y", "--no-progress"}
	if !reflect.DeepEqual(mock.ranArgs, want) {
		t.Errorf("args = %v, want %v", mock.ranArgs, want)
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "git.install", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "git.install", "-y"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
