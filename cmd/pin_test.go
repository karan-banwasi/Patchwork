package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestPinUnpinPinnedCmds(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	// 1. Pinned initially empty
	buf := new(bytes.Buffer)
	cmd1 := NewRootCmd()
	cmd1.SetOut(buf)
	cmd1.SetArgs([]string{"pinned"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("unexpected error on pinned: %v", err)
	}
	if !strings.Contains(buf.String(), "No packages are currently pinned.") {
		t.Errorf("expected empty pinned message, got: %s", buf.String())
	}

	// 2. Pin multiple packages at once
	buf.Reset()
	cmd2 := NewRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"pin", "Git.Git", "Microsoft.VisualStudioCode"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("unexpected error on pin: %v", err)
	}
	if !strings.Contains(buf.String(), `Pinned package "Git.Git"`) || !strings.Contains(buf.String(), `Pinned package "Microsoft.VisualStudioCode"`) {
		t.Errorf("unexpected pin output: %s", buf.String())
	}

	// 3. List pinned
	buf.Reset()
	cmd3 := NewRootCmd()
	cmd3.SetOut(buf)
	cmd3.SetArgs([]string{"pinned"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("unexpected error on pinned: %v", err)
	}
	if !strings.Contains(buf.String(), "Git.Git") || !strings.Contains(buf.String(), "Microsoft.VisualStudioCode") {
		t.Errorf("expected Git.Git and Microsoft.VisualStudioCode in pinned list, got: %s", buf.String())
	}

	// 4. Unpin multiple packages
	buf.Reset()
	cmd4 := NewRootCmd()
	cmd4.SetOut(buf)
	cmd4.SetArgs([]string{"unpin", "git.git", "microsoft.visualstudiocode"})
	if err := cmd4.Execute(); err != nil {
		t.Fatalf("unexpected error on unpin: %v", err)
	}
	if !strings.Contains(buf.String(), `Unpinned package "git.git"`) || !strings.Contains(buf.String(), `Unpinned package "microsoft.visualstudiocode"`) {
		t.Errorf("unexpected unpin output: %s", buf.String())
	}
}

func TestFilterOutPinned_CorruptConfigFailsClosed(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	if err := os.WriteFile(tmpCfg, []byte("{ corrupt: json"), 0600); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	updates := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
	}

	_, _, err := filterOutPinned(updates)
	if err == nil {
		t.Fatalf("expected error from filterOutPinned on corrupt config, got nil")
	}
	if !strings.Contains(err.Error(), "failed to load configuration") {
		t.Errorf("expected failed to load configuration error, got: %v", err)
	}
}

func TestFilterOutPinned_NonExistentConfigSucceeds(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "nonexistent", "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	updates := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
	}

	filtered, skipped, err := filterOutPinned(updates)
	if err != nil {
		t.Fatalf("unexpected error when config does not exist: %v", err)
	}
	if len(filtered) != 1 || skipped != 0 {
		t.Errorf("expected 1 filtered and 0 skipped, got filtered=%d, skipped=%d", len(filtered), skipped)
	}
}

func TestPinnedCmd_JSON(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	// Pin items
	buf := new(bytes.Buffer)
	pinCmd := NewRootCmd()
	pinCmd.SetOut(buf)
	pinCmd.SetArgs([]string{"pin", "Git.Git", "scoop:curl"})
	if err := pinCmd.Execute(); err != nil {
		t.Fatalf("pin execute: %v", err)
	}

	buf.Reset()
	pinnedCmd := NewRootCmd()
	pinnedCmd.SetOut(buf)
	pinnedCmd.SetArgs([]string{"pinned", "--json"})
	if err := pinnedCmd.Execute(); err != nil {
		t.Fatalf("pinned --json execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"schema_version": "1.0.0"`) {
		t.Errorf("expected schema_version in json output, got: %s", out)
	}
	if !strings.Contains(out, `"count": 2`) {
		t.Errorf("expected count 2 in json output, got: %s", out)
	}
	if !strings.Contains(out, `"manager": "scoop"`) {
		t.Errorf("expected scoop manager in json output, got: %s", out)
	}
}

func TestPinCmd_RicherFlagsAndOutput(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	// Pin with rich metadata
	buf := new(bytes.Buffer)
	pinCmd := NewRootCmd()
	pinCmd.SetOut(buf)
	pinCmd.SetArgs([]string{"pin", "npm:node", "--version", "<23", "--reason", "breaks database driver", "--expires", "2029-12-31"})
	if err := pinCmd.Execute(); err != nil {
		t.Fatalf("pin execute: %v", err)
	}
	pinOutput := buf.String()
	if !strings.Contains(pinOutput, `<23`) || !strings.Contains(pinOutput, "breaks database driver") {
		t.Errorf("expected rich pin output details, got: %s", pinOutput)
	}

	// Verify text output in pinned
	buf.Reset()
	pinnedCmd := NewRootCmd()
	pinnedCmd.SetOut(buf)
	pinnedCmd.SetArgs([]string{"pinned"})
	if err := pinnedCmd.Execute(); err != nil {
		t.Fatalf("pinned execute: %v", err)
	}
	pinnedText := buf.String()
	if !strings.Contains(pinnedText, "node") || !strings.Contains(pinnedText, "[constraint: <23]") || !strings.Contains(pinnedText, "[reason: breaks database driver]") {
		t.Errorf("expected constraint and reason in text output, got: %s", pinnedText)
	}

	// Verify JSON output in pinned --json
	buf.Reset()
	jsonCmd := NewRootCmd()
	jsonCmd.SetOut(buf)
	jsonCmd.SetArgs([]string{"pinned", "--json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("pinned --json execute: %v", err)
	}
	jsonOut := buf.String()
	if !strings.Contains(jsonOut, `"version": "<23"`) || !strings.Contains(jsonOut, `"reason": "breaks database driver"`) {
		t.Errorf("expected version and reason in JSON output, got: %s", jsonOut)
	}
	if !strings.Contains(jsonOut, `"expired": false`) {
		t.Errorf("expected expired false in JSON output, got: %s", jsonOut)
	}

	// Unpin
	buf.Reset()
	unpinCmd := NewRootCmd()
	unpinCmd.SetOut(buf)
	unpinCmd.SetArgs([]string{"unpin", "npm:node"})
	if err := unpinCmd.Execute(); err != nil {
		t.Fatalf("unpin execute: %v", err)
	}
	if !strings.Contains(buf.String(), `Unpinned package "npm:node"`) {
		t.Errorf("expected unpinned output, got: %s", buf.String())
	}
}
