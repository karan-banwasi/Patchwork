package cmd

import (
	"strings"
	"testing"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/karan-banwasi/patchwork/internal/pm"
)

func TestSurveyMultiSelectTemplateRendering(t *testing.T) {
	initSurveyTemplates()
	tmpl, err := template.New("prompt").Funcs(core.TemplateFuncsWithColor).Parse(survey.MultiSelectQuestionTemplate)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	opts := []string{
		"Outlook for Windows   Microsoft.Outlook   1.2026.811.200   1.2026.812.100   winget",
		"Git                   Git.Git             2.55.0.2         2.55.0.3         winget",
	}

	// 0 selected
	var buf0 strings.Builder
	err = tmpl.Execute(&buf0, survey.MultiSelectTemplateData{
		MultiSelect: survey.MultiSelect{Message: "Select packages to upgrade:", Options: opts},
		Checked:     map[int]bool{},
		Answer:      "",
		ShowAnswer:  true,
		Config:      &survey.PromptConfig{Icons: survey.IconSet{Question: survey.Icon{Text: "?"}}},
	})
	if err != nil {
		t.Fatalf("failed to render 0 selected: %v", err)
	}
	if strings.Contains(buf0.String(), "Outlook") {
		t.Errorf("unexpected output for 0 selections: %s", buf0.String())
	}

	// 1 selected
	var buf1 strings.Builder
	err = tmpl.Execute(&buf1, survey.MultiSelectTemplateData{
		MultiSelect: survey.MultiSelect{Message: "Select packages to upgrade:", Options: opts},
		Checked:     map[int]bool{0: true},
		Answer:      opts[0],
		ShowAnswer:  true,
		Config:      &survey.PromptConfig{Icons: survey.IconSet{Question: survey.Icon{Text: "?"}}},
	})
	if err != nil {
		t.Fatalf("failed to render 1 selected: %v", err)
	}
	if !strings.Contains(buf1.String(), "\n  ") || !strings.Contains(buf1.String(), opts[0]) {
		t.Errorf("expected package on new line with indent, got: %s", buf1.String())
	}

	// 2 selected
	var buf2 strings.Builder
	err = tmpl.Execute(&buf2, survey.MultiSelectTemplateData{
		MultiSelect: survey.MultiSelect{Message: "Select packages to upgrade:", Options: opts},
		Checked:     map[int]bool{0: true, 1: true},
		Answer:      strings.Join(opts, ", "),
		ShowAnswer:  true,
		Config:      &survey.PromptConfig{Icons: survey.IconSet{Question: survey.Icon{Text: "?"}}},
	})
	if err != nil {
		t.Fatalf("failed to render 2 selected: %v", err)
	}
	lines := strings.Split(buf2.String(), "\n")
	var packageLines []string
	for _, l := range lines {
		if strings.Contains(l, "winget") {
			packageLines = append(packageLines, l)
		}
	}
	if len(packageLines) != 2 {
		t.Errorf("expected 2 package lines, got %d. Output:\n%s", len(packageLines), buf2.String())
	}

	// ShowAnswer = false (Interactive options list)
	var bufInteractive strings.Builder
	pageEntries := []core.OptionAnswer{
		{Index: 0, Value: opts[0]},
		{Index: 1, Value: opts[1]},
	}
	err = tmpl.Execute(&bufInteractive, survey.MultiSelectTemplateData{
		MultiSelect: survey.MultiSelect{Message: "Select packages to upgrade:", Options: opts},
		PageEntries: pageEntries,
		Checked:     map[int]bool{0: true},
		ShowAnswer:  false,
		Config:      &survey.PromptConfig{Icons: survey.IconSet{Question: survey.Icon{Text: "?"}}},
	})
	if err != nil {
		t.Fatalf("failed to render interactive prompt: %v", err)
	}
	t.Logf("Interactive output:\n%s", bufInteractive.String())

	// Test core.RunTemplate
	res, _, err := core.RunTemplate(survey.MultiSelectQuestionTemplate, survey.MultiSelectTemplateData{
		MultiSelect: survey.MultiSelect{Message: "Select packages to upgrade:", Options: opts},
		PageEntries: pageEntries,
		Checked:     map[int]bool{0: true},
		ShowAnswer:  true,
		Answer:      opts[0],
		Config:      &survey.PromptConfig{Icons: survey.IconSet{Question: survey.Icon{Text: "?"}}},
	})
	if err != nil {
		t.Fatalf("core.RunTemplate failed: %v", err)
	}
	t.Logf("core.RunTemplate output: %s", res)
}

func TestUpgradeCmd_MutuallyExclusiveAllAndArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--all", "Some.Package"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when combining --all and package argument, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUpgradeCmd_UnknownManager(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "Some.Package", "-m", "unknown-mgr"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown manager, got nil")
	}
	if !strings.Contains(err.Error(), "unknown package manager") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUpgradeCmd_MutuallyExclusiveByManagerAndArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--by-manager", "Some.Package"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when combining --by-manager and package argument, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrderSelectedPackages_DefersShellAndTerminal(t *testing.T) {
	input := []pm.PackageUpdate{
		{Id: "Microsoft.PowerShell", Name: "PowerShell", ManagerName: "winget"},
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
		{Id: "Microsoft.WindowsTerminal", Name: "Windows Terminal", ManagerName: "winget"},
		{Id: "OpenJS.NodeJS.LTS", Name: "Node.js LTS", ManagerName: "winget"},
	}

	ordered := orderSelectedPackages(input)
	expectedOrder := []string{
		"Git.Git",
		"OpenJS.NodeJS.LTS",
		"Microsoft.PowerShell",
		"Microsoft.WindowsTerminal",
	}

	if len(ordered) != len(expectedOrder) {
		t.Fatalf("expected %d packages, got %d", len(expectedOrder), len(ordered))
	}
	for i, exp := range expectedOrder {
		if ordered[i].Id != exp {
			t.Errorf("at index %d: expected %q, got %q", i, exp, ordered[i].Id)
		}
	}
}

func TestSplitPinnedTargets_SkipsPinnedUnlessForced(t *testing.T) {
	tmpCfg := t.TempDir() + "/config.json"
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"pin", "winget:Git.Git"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to pin: %v", err)
	}

	targets := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
		{Id: "Git.Git", Name: "Git", ManagerName: "scoop"},
	}

	allowed, skipped := splitPinnedTargets(targets, false)
	if len(allowed) != 1 || allowed[0].ManagerName != "scoop" {
		t.Errorf("expected only scoop Git.Git to be allowed, got: %+v", allowed)
	}
	if len(skipped) != 1 || skipped[0].status != "Skipped (pinned)" {
		t.Errorf("expected winget Git.Git to be Skipped (pinned), got: %+v", skipped)
	}

	allowedForced, skippedForced := splitPinnedTargets(targets, true)
	if len(allowedForced) != 2 || len(skippedForced) != 0 {
		t.Errorf("expected all targets allowed when ignorePins=true, got allowed=%d, skipped=%d", len(allowedForced), len(skippedForced))
	}
}
