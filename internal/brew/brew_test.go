package brew

import (
	"bytes"
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

func TestBrewManager_NameAndAliases(t *testing.T) {
	mgr := NewBrewManager()
	if mgr.Name() != "brew" {
		t.Fatalf("expected Name() to be 'brew', got %q", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "homebrew" {
		t.Fatalf("expected Aliases() to contain 'homebrew', got %v", aliases)
	}
}

func TestBrewManager_ValidateID(t *testing.T) {
	mgr := NewBrewManager()

	valid := []string{
		"wget",
		"python@3.11",
		"visual-studio-code",
		"homebrew/cask/google-chrome",
		"gtk+3",
		"libsigc++",
		"hashicorp/tap/terraform",
	}
	for _, id := range valid {
		if err := mgr.ValidateID(id); err != nil {
			t.Errorf("expected valid ID %q, got error: %v", id, err)
		}
	}

	invalid := []string{
		"",
		"-flag",
		"../escape",
		"pkg with space",
		"pkg;rm",
		"pkg|pipe",
		"pkg\nnewline",
	}
	for _, id := range invalid {
		if err := mgr.ValidateID(id); err == nil {
			t.Errorf("expected invalid ID %q to fail validation", id)
		}
	}
}

func TestParseBrewOutdatedJSON(t *testing.T) {
	jsonInput := `{
  "formulae": [
    {
      "name": "wget",
      "installed_versions": ["1.21.3"],
      "current_version": "1.21.4",
      "pinned": false,
      "pinned_version": null
    },
    {
      "name": "node@20",
      "installed_versions": ["20.10.0"],
      "current_version": "20.11.0",
      "pinned": false,
      "pinned_version": null
    },
    {
      "name": "up-to-date",
      "installed_versions": ["1.0.0"],
      "current_version": "1.0.0",
      "pinned": false,
      "pinned_version": null
    }
  ],
  "casks": [
    {
      "token": "google-chrome",
      "name": ["Google Chrome"],
      "installed_versions": ["122.0.6261.112"],
      "current_version": "123.0.6312.58",
      "pinned": false,
      "pinned_version": null
    },
    {
      "token": "visual-studio-code",
      "name": "Visual Studio Code",
      "installed_versions": ["1.85.0"],
      "current_version": "1.86.0",
      "pinned": false,
      "pinned_version": null
    }
  ]
}`

	updates, err := parseBrewOutdatedJSON(jsonInput)
	if err != nil {
		t.Fatalf("unexpected error parsing brew JSON: %v", err)
	}

	// Expect 4 updates (up-to-date is excluded because version == current_version)
	if len(updates) != 4 {
		t.Fatalf("expected 4 updates, got %d", len(updates))
	}

	// Case-insensitive alphabetical order by display Name:
	// 1. Google Chrome (cask)
	// 2. node@20 (homebrew)
	// 3. Visual Studio Code (cask)
	// 4. wget (homebrew)
	if updates[0].Name != "Google Chrome" || updates[0].Id != "google-chrome" || updates[0].Source != "cask" {
		t.Errorf("unexpected update[0]: %+v", updates[0])
	}
	if updates[1].Name != "node@20" || updates[1].Version != "20.10.0" || updates[1].AvailableVersion != "20.11.0" {
		t.Errorf("unexpected update[1]: %+v", updates[1])
	}
	if updates[2].Name != "Visual Studio Code" || updates[2].Id != "visual-studio-code" {
		t.Errorf("unexpected update[2]: %+v", updates[2])
	}
	if updates[3].Name != "wget" || updates[3].Version != "1.21.3" || updates[3].AvailableVersion != "1.21.4" {
		t.Errorf("unexpected update[3]: %+v", updates[3])
	}
}

func TestParseBrewOutdatedJSON_SkipsPinned(t *testing.T) {
	jsonInput := `{
  "formulae": [
    {
      "name": "pinned-pkg",
      "installed_versions": ["1.0.0"],
      "current_version": "2.0.0",
      "pinned": true,
      "pinned_version": "1.0.0"
    },
    {
      "name": "active-pkg",
      "installed_versions": ["1.0.0"],
      "current_version": "2.0.0",
      "pinned": false,
      "pinned_version": null
    }
  ],
  "casks": [
    {
      "token": "pinned-cask",
      "name": "Pinned Cask",
      "installed_versions": ["1.0.0"],
      "current_version": "2.0.0",
      "pinned": true,
      "pinned_version": "1.0.0"
    }
  ]
}`

	updates, err := parseBrewOutdatedJSON(jsonInput)
	if err != nil {
		t.Fatalf("unexpected error parsing brew JSON: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update (pinned packages skipped), got %d", len(updates))
	}
	if updates[0].Id != "active-pkg" {
		t.Fatalf("expected active-pkg, got %s", updates[0].Id)
	}
}

func TestParseBrewOutdatedJSON_EmptyAndInvalid(t *testing.T) {
	res, err := parseBrewOutdatedJSON("")
	if err != nil || len(res) != 0 {
		t.Errorf("expected empty slice for empty string, got: %v, err: %v", res, err)
	}

	res, err = parseBrewOutdatedJSON("{}")
	if err != nil || len(res) != 0 {
		t.Errorf("expected empty slice for {}, got: %v, err: %v", res, err)
	}

	_, err = parseBrewOutdatedJSON("{invalid-json")
	if err == nil {
		t.Errorf("expected error for invalid JSON")
	}
}

func TestBrewManager_UpgradePackages(t *testing.T) {
	var invokedArgs []string
	mgr := NewBrewManager()
	mgr.exePath = "brew"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			invokedArgs = args
			return nil
		},
	}

	// Single upgrade
	if err := mgr.UpgradePackage(context.Background(), "wget"); err != nil {
		t.Fatalf("unexpected error on UpgradePackage: %v", err)
	}
	if len(invokedArgs) != 2 || invokedArgs[0] != "upgrade" || invokedArgs[1] != "wget" {
		t.Errorf("unexpected args for single upgrade: %v", invokedArgs)
	}

	// Bulk upgrade
	if err := mgr.UpgradePackages(context.Background(), []string{"wget", "node@20"}); err != nil {
		t.Fatalf("unexpected error on UpgradePackages: %v", err)
	}
	if len(invokedArgs) != 3 || invokedArgs[0] != "upgrade" || invokedArgs[1] != "wget" || invokedArgs[2] != "node@20" {
		t.Errorf("unexpected args for bulk upgrade: %v", invokedArgs)
	}

	// Validation failure
	if err := mgr.UpgradePackage(context.Background(), "-invalid"); err == nil {
		t.Fatalf("expected validation error for invalid ID")
	}
}

func TestBrewManager_Refresh(t *testing.T) {
	var invokedArgs []string
	mgr := NewBrewManager()
	mgr.exePath = "brew"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			invokedArgs = args
			return nil
		},
	}

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("unexpected error on Refresh: %v", err)
	}
	if len(invokedArgs) != 1 || invokedArgs[0] != "update" {
		t.Errorf("unexpected args for update: %v", invokedArgs)
	}
}

func TestBrewManager_Cleanup(t *testing.T) {
	var invokedArgs []string
	var outBuf bytes.Buffer
	mgr := NewBrewManager()
	mgr.exePath = "brew"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			invokedArgs = args
			return nil
		},
	}

	if err := mgr.Cleanup(context.Background(), &outBuf, io.Discard); err != nil {
		t.Fatalf("unexpected error on Cleanup: %v", err)
	}
	if len(invokedArgs) != 1 || invokedArgs[0] != "cleanup" {
		t.Errorf("unexpected args for cleanup: %v", invokedArgs)
	}
	if !strings.Contains(outBuf.String(), "Cleaning up Homebrew cache") {
		t.Errorf("expected cleanup header, got: %s", outBuf.String())
	}
}

func TestBrewManager_SetIO(t *testing.T) {
	mgr := NewBrewManager()
	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer
	mgr.SetIO(&in, &out, &errOut)
	if mgr.stdin != &in || mgr.stdout != &out || mgr.stderr != &errOut {
		t.Errorf("SetIO did not correctly assign streams")
	}
}

func TestBrewManager_RunnerError(t *testing.T) {
	mgr := NewBrewManager()
	mgr.exePath = "brew"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return errors.New("command failed")
		},
	}

	if err := mgr.UpgradePackage(context.Background(), "wget"); err == nil {
		t.Fatalf("expected error from runner")
	}
}
