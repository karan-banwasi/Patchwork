package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var updateDocs = flag.Bool("update", false, "update CLI markdown documentation in docs/cli")

// genMarkdownTree produces Cobra-standard markdown documentation tree for a command hierarchy.
func genMarkdownTree(cmd *cobra.Command, dir string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := genMarkdownTree(c, dir); err != nil {
			return err
		}
	}

	basename := strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
	filename := filepath.Join(dir, basename)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return genMarkdownCustom(cmd, f)
}

func genMarkdownCustom(cmd *cobra.Command, w io.Writer) error {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	buf := new(bytes.Buffer)
	name := cmd.CommandPath()

	buf.WriteString("## " + name + "\n\n")
	buf.WriteString(cmd.Short + "\n\n")
	if len(cmd.Long) > 0 {
		buf.WriteString("### Synopsis\n\n")
		buf.WriteString(cmd.Long + "\n\n")
	}

	if cmd.Runnable() {
		fmt.Fprintf(buf, "```\n%s\n```\n\n", cmd.UseLine())
	}

	if len(cmd.Example) > 0 {
		buf.WriteString("### Examples\n\n")
		fmt.Fprintf(buf, "```\n%s\n```\n\n", cmd.Example)
	}

	if err := printOptions(buf, cmd); err != nil {
		return err
	}

	if hasSeeAlso(cmd) {
		buf.WriteString("### SEE ALSO\n\n")
		if cmd.HasParent() {
			parent := cmd.Parent()
			pname := parent.CommandPath()
			link := strings.ReplaceAll(pname, " ", "_") + ".md"
			fmt.Fprintf(buf, "* [%s](%s)\t - %s\n", pname, link, parent.Short)
		}

		children := cmd.Commands()
		sort.Slice(children, func(i, j int) bool {
			return children[i].Name() < children[j].Name()
		})

		for _, child := range children {
			if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
				continue
			}
			cname := name + " " + child.Name()
			link := strings.ReplaceAll(cname, " ", "_") + ".md"
			fmt.Fprintf(buf, "* [%s](%s)\t - %s\n", cname, link, child.Short)
		}
		buf.WriteString("\n")
	}

	_, err := buf.WriteTo(w)
	return err
}

func printOptions(buf *bytes.Buffer, cmd *cobra.Command) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(buf)
	if flags.HasAvailableFlags() {
		buf.WriteString("### Options\n\n```\n")
		flags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	parentFlags := cmd.InheritedFlags()
	parentFlags.SetOutput(buf)
	if parentFlags.HasAvailableFlags() {
		buf.WriteString("### Options inherited from parent commands\n\n```\n")
		parentFlags.PrintDefaults()
		buf.WriteString("```\n\n")
	}
	return nil
}

func hasSeeAlso(cmd *cobra.Command) bool {
	if cmd.HasParent() {
		return true
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		return true
	}
	return false
}

func TestCLIDocumentationUpToDate(t *testing.T) {
	docsDir := filepath.Join("..", "docs", "cli")
	tempDir := t.TempDir()

	rootCmd := NewRootCmd()
	rootCmd.DisableAutoGenTag = true

	if err := genMarkdownTree(rootCmd, tempDir); err != nil {
		t.Fatalf("failed to generate markdown documentation: %v", err)
	}

	generatedFiles, err := filepath.Glob(filepath.Join(tempDir, "*.md"))
	if err != nil {
		t.Fatalf("failed to glob generated doc files: %v", err)
	}
	if len(generatedFiles) == 0 {
		t.Fatalf("no markdown doc files were generated in %s", tempDir)
	}

	if *updateDocs {
		if err := os.MkdirAll(docsDir, 0755); err != nil {
			t.Fatalf("failed to create docsDir %s: %v", docsDir, err)
		}
		for _, genFile := range generatedFiles {
			destFile := filepath.Join(docsDir, filepath.Base(genFile))
			content, err := os.ReadFile(genFile)
			if err != nil {
				t.Fatalf("failed to read generated file %s: %v", genFile, err)
			}
			if err := os.WriteFile(destFile, content, 0644); err != nil {
				t.Fatalf("failed to write %s: %v", destFile, err)
			}
		}
		return
	}

	// Compare generated docs against committed docs
	for _, genFile := range generatedFiles {
		baseName := filepath.Base(genFile)
		committedFile := filepath.Join(docsDir, baseName)

		genContent, err := os.ReadFile(genFile)
		if err != nil {
			t.Fatalf("failed to read generated file %s: %v", genFile, err)
		}

		committedContent, err := os.ReadFile(committedFile)
		if err != nil {
			t.Fatalf("missing CLI doc file %s. Run 'go test ./cmd -update' to generate.", committedFile)
		}

		if string(genContent) != string(committedContent) {
			t.Errorf("CLI doc %s is out of date. Run 'go test ./cmd -update' to regenerate.", baseName)
		}
	}
}
