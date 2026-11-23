package choco

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChoco_ListInstalled(t *testing.T) {
	mock := &mockChocoRunner{
		output: "Chocolatey v2.4.0\ngit.install|2.45.0\n7zip|24.06\n2 packages installed.\n",
	}
	mgr := NewChocoManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "7zip" || pkgs[0].Version != "24.06" || pkgs[0].ManagerName != "choco" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestChoco_ListInstalled_Error(t *testing.T) {
	mgr := NewChocoManager()
	mgr.runner = &mockChocoRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChoco_InstallPackage(t *testing.T) {
	mock := &mockChocoRunner{}
	mgr := NewChocoManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "git.install", "2.45.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	joined := strings.Join(mock.ranArgs, " ")
	if joined != "install git.install -y --no-progress --version 2.45.0" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git.install", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
