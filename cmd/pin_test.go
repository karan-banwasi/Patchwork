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
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"pinned"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error on pinned: %v", err)
	}
	if !strings.Contains(buf.String(), "No packages are currently pinned.") {
		t.Errorf("expected empty pinned message, got: %s", buf.String())
	}

	// 2. Pin a package
	buf.Reset()
	rootCmd.SetArgs([]string{"pin", "Git.Git"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error on pin: %v", err)
	}
	if !strings.Contains(buf.String(), `Pinned package "Git.Git"`) {
		t.Errorf("unexpected pin output: %s", buf.String())
	}

	// 3. List pinned
	buf.Reset()
	rootCmd.SetArgs([]string{"pinned"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error on pinned: %v", err)
	}
	if !strings.Contains(buf.String(), "Git.Git") {
		t.Errorf("expected Git.Git in pinned list, got: %s", buf.String())
	}

	// 4. Unpin
	buf.Reset()
	rootCmd.SetArgs([]string{"unpin", "git.git"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error on unpin: %v", err)
	}
	if !strings.Contains(buf.String(), `Unpinned package "git.git"`) {
		t.Errorf("unexpected unpin output: %s", buf.String())
	}
}
