package cargo

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type mockRunner struct {
	runFunc func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

func (m *mockRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, name, args, stdin, stdout, stderr)
	}
	return nil
}

func TestParseCargoInstallList(t *testing.T) {
	sample := `bat v0.24.0:
    bat.exe
ripgrep v14.1.0:
    rg.exe
my-local-tool v0.1.0 (C:\projects\my-local-tool):
    my-local-tool.exe
`
	crates := parseCargoInstallList(sample)
	if len(crates) != 2 {
		t.Fatalf("expected 2 crates.io packages (excluding local path crate), got %d", len(crates))
	}
	if crates[0].Id != "bat" || crates[0].Version != "0.24.0" {
		t.Errorf("unexpected crate[0]: %+v", crates[0])
	}
	if crates[1].Id != "ripgrep" || crates[1].Version != "14.1.0" {
		t.Errorf("unexpected crate[1]: %+v", crates[1])
	}
}

func TestCargo_ListFailurePropagatesError(t *testing.T) {
	binDir := t.TempDir()
	binName := "cargo"
	if runtime.GOOS == "windows" {
		binName = "cargo.exe"
	}
	if err := os.WriteFile(filepath.Join(binDir, binName), []byte(""), 0755); err != nil {
		t.Fatalf("failed to create dummy cargo: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewCargoManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "install" {
				stderr.Write([]byte("cargo config corrupted"))
				return errors.New("exit status 1")
			}
			return nil
		},
	}

	_, _, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err == nil {
		t.Fatalf("expected error when cargo install --list fails, got nil")
	}
	if !strings.Contains(err.Error(), "cargo config corrupted") {
		t.Errorf("expected error message to contain stderr output, got: %v", err)
	}
}
