package flatpak

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type mockProfileRunner struct {
	ranArgs []string
	stdout  string
	stderr  string
	err     error
}

func (m *mockProfileRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	m.ranArgs = append([]string(nil), args...)
	if stdout != nil && m.stdout != "" {
		_, _ = stdout.Write([]byte(m.stdout))
	}
	if stderr != nil && m.stderr != "" {
		_, _ = stderr.Write([]byte(m.stderr))
	}
	return m.err
}

func TestFlatpak_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "org.mozilla.firefox\tFirefox\t126.0\ncom.spotify.Client\tSpotify\t1.2.3\n",
	}
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "com.spotify.Client" || pkgs[0].Name != "Spotify" || pkgs[0].Version != "1.2.3" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestFlatpak_ListInstalled_Error(t *testing.T) {
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFlatpak_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "org.mozilla.firefox", ""); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install -y org.mozilla.firefox" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "bad", ""); err == nil {
		t.Error("expected error for invalid Flatpak ID")
	}
	if err := mgr.InstallPackage(context.Background(), "org.mozilla.firefox", "--user"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
