package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing version command: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "pw version") {
		t.Errorf("expected output to contain 'pw version', got %q", output)
	}
}
