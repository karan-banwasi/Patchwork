package uv

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

func TestParseUvToolList(t *testing.T) {
	sample := `ruff v0.6.1
- ruff.exe
httpie v3.2.2
- http.exe
- https.exe
black v24.4.2
- black.exe
`
	tools := parseUvToolList(sample)
	if len(tools) != 3 {
		t.Fatalf("expected 3 uv tools, got %d", len(tools))
	}
	if tools[0].Id != "ruff" || tools[0].Version != "0.6.1" {
		t.Errorf("unexpected tool[0]: %+v", tools[0])
	}
	if tools[1].Id != "httpie" || tools[1].Version != "3.2.2" {
		t.Errorf("unexpected tool[1]: %+v", tools[1])
	}
	if tools[2].Id != "black" || tools[2].Version != "24.4.2" {
		t.Errorf("unexpected tool[2]: %+v", tools[2])
	}
}

func TestUv_ListFailurePropagatesError(t *testing.T) {
	binDir := t.TempDir()
	binName := "uv"
	if runtime.GOOS == "windows" {
		binName = "uv.exe"
	}
	if err := os.WriteFile(filepath.Join(binDir, binName), []byte(""), 0755); err != nil {
		t.Fatalf("failed to create dummy uv: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr := NewUvManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "tool" && args[1] == "list" {
				stderr.Write([]byte("failed to parse environment"))
				return errors.New("exit status 1")
			}
			return nil
		},
	}

	_, _, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err == nil {
		t.Fatalf("expected error when uv tool list fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse environment") {
		t.Errorf("expected error message to contain stderr output, got: %v", err)
	}
}
