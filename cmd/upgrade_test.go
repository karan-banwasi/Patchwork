package cmd

import (
	"strings"
	"testing"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
)

func TestSurveyMultiSelectTemplateRendering(t *testing.T) {
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



