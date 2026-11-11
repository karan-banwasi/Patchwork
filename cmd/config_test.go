package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCommands(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	// 1. Initial config get (unset)
	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "get", "defaults.timeout"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get: %v", err)
	}
	if !strings.Contains(buf.String(), "<not set>") {
		t.Errorf("expected <not set>, got: %s", buf.String())
	}

	// 2. Set defaults.timeout
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "set", "defaults.timeout", "45s"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if !strings.Contains(buf.String(), "Set defaults.timeout = 45s") {
		t.Errorf("unexpected set output: %s", buf.String())
	}

	// 3. Get defaults.timeout
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "get", "defaults.timeout"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get: %v", err)
	}
	if !strings.Contains(buf.String(), "45s") {
		t.Errorf("expected 45s, got: %s", buf.String())
	}

	// 4. Set boolean flags
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "set", "defaults.accept-agreements", "true"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set bool: %v", err)
	}

	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "get", "defaults.accept_agreements"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get bool: %v", err)
	}
	if !strings.Contains(buf.String(), "true") {
		t.Errorf("expected true, got: %s", buf.String())
	}

	// 5. Set disabled_managers
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "set", "disabled_managers", "cargo,psresource"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set disabled_managers: %v", err)
	}

	// 6. Config list / show
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config list: %v", err)
	}
	listOut := buf.String()
	if !strings.Contains(listOut, "45s") || !strings.Contains(listOut, "cargo") || !strings.Contains(listOut, "psresource") {
		t.Errorf("expected full config in list output, got: %s", listOut)
	}

	// 7. Config edit in non-interactive environment returns informative error
	buf.Reset()
	cmd = NewRootCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "edit"})
	err := cmd.Execute()
	if err == nil {
		t.Errorf("expected error for config edit in non-interactive shell")
	}
	if !strings.Contains(err.Error(), "cannot open editor in non-interactive environment") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestConfigValidationErrors(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	// Invalid key
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"config", "get", "unknown_key"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for unknown key")
	}

	// Invalid duration
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"config", "set", "defaults.timeout", "not-a-duration"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for invalid duration")
	}

	// Invalid boolean
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"config", "set", "defaults.no-refresh", "not-a-bool"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for invalid boolean")
	}

	// Invalid manager name
	cmd = NewRootCmd()
	cmd.SetArgs([]string{"config", "set", "disabled_managers", "fake-pm"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for invalid manager name")
	}
}
