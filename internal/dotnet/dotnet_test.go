package dotnet

import (
	"context"
	"errors"
	"io"
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

func TestParseDotnetToolListAndSearch(t *testing.T) {
	listOut := `Package Id                Version      Commands
---------------------------------------------------
dotnet-ef                 8.0.4        dotnet-ef
csharpier                 0.28.2       dotnet-csharpier
`
	tools := parseDotnetToolList(listOut)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Id != "dotnet-ef" || tools[0].Version != "8.0.4" {
		t.Errorf("unexpected tool 0: %+v", tools[0])
	}

	searchOut := `Package ID      Latest Version      Authors      Downloads      Verified
----------------------------------------------------------------------------
dotnet-ef       8.0.8               Microsoft    50000000       x
`
	latest := parseDotnetToolSearchLatest(searchOut, "dotnet-ef")
	if latest != "8.0.8" {
		t.Errorf("expected latest 8.0.8, got %q", latest)
	}
}

func TestDotnet_ListFailurePropagatesError(t *testing.T) {
	mgr := NewDotnetManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 1 && args[0] == "tool" && args[1] == "list" {
				stderr.Write([]byte("dotnet host not found"))
				return errors.New("exit status 1")
			}
			return nil
		},
	}

	_, _, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err == nil {
		t.Fatalf("expected error when dotnet tool list fails, got nil")
	}
	if !strings.Contains(err.Error(), "dotnet host not found") {
		t.Errorf("expected error message to contain stderr output, got: %v", err)
	}
}

func TestDotnet_SearchFailureGeneratesWarning(t *testing.T) {
	mgr := NewDotnetManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 1 && args[0] == "tool" && args[1] == "list" {
				stdout.Write([]byte(`Package Id                Version      Commands
---------------------------------------------------
tool-ok                   1.0.0        tool-ok
tool-fail                 1.0.0        tool-fail
`))
				return nil
			}
			if len(args) > 1 && args[0] == "tool" && args[1] == "search" {
				target := args[2]
				if target == "tool-fail" {
					stderr.Write([]byte("feed offline"))
					return errors.New("search error")
				}
				stdout.Write([]byte(`Package ID      Latest Version
------------------------------
tool-ok         2.0.0
`))
				return nil
			}
			return nil
		},
	}

	updates, warnings, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "tool-ok" {
		t.Errorf("expected 1 update for tool-ok, got: %+v", updates)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "1 of 2 tools could not be checked") {
		t.Errorf("expected 1 warning about partial failure, got: %v", warnings)
	}
}

func TestDotnet_IsAvailable(t *testing.T) {
	mgr := NewDotnetManager()

	// When dotnet executable path is available:
	if _, err := dotnetPath(); err != nil {
		t.Skip("dotnet executable not found on host, skipping IsAvailable host test")
	}

	// Case 1: SDKs present
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) == 1 && args[0] == "--list-sdks" {
				stdout.Write([]byte("8.0.404 [C:\\Program Files\\dotnet\\sdk]\n"))
				return nil
			}
			return nil
		},
	}
	if !mgr.IsAvailable(context.Background()) {
		t.Errorf("expected IsAvailable to return true when SDK is present")
	}

	// Case 2: No SDKs present (empty output)
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) == 1 && args[0] == "--list-sdks" {
				return nil
			}
			return nil
		},
	}
	if mgr.IsAvailable(context.Background()) {
		t.Errorf("expected IsAvailable to return false when no SDKs are installed")
	}

	// Case 3: Error running --list-sdks
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return errors.New("command failed")
		},
	}
	if mgr.IsAvailable(context.Background()) {
		t.Errorf("expected IsAvailable to return false on execution error")
	}
}

func TestDotnet_NoSDKError_ToleratedInUpdateCheck(t *testing.T) {
	mgr := NewDotnetManager()
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 1 && args[0] == "tool" && args[1] == "list" {
				stderr.Write([]byte(`The command could not be loaded, possibly because:
  * You intended to execute a .NET application:
      The application 'tool' does not exist or is not a managed .dll or .exe.
  * You intended to execute a .NET SDK command:
      No .NET SDKs were found.

Download a .NET SDK:
https://aka.ms/dotnet/download`))
				return errors.New("exit status 0x8000809b")
			}
			return nil
		},
	}

	updates, warnings, err := mgr.GetAvailableUpdatesWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("expected no error when no SDK is found, got: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(updates))
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestDotnet_Version(t *testing.T) {
	mgr := NewDotnetManager()
	if _, err := dotnetPath(); err != nil {
		t.Skip("dotnet executable not found on host, skipping Version host test")
	}

	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) == 1 && args[0] == "--version" {
				stdout.Write([]byte("8.0.404\n"))
				return nil
			}
			return nil
		},
	}
	ver, err := mgr.Version(context.Background())
	if err != nil {
		t.Fatalf("unexpected error getting version: %v", err)
	}
	if ver != "8.0.404" {
		t.Errorf("expected version 8.0.404, got %q", ver)
	}
}
