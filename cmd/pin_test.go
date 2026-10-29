package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
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
