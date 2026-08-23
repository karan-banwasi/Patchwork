package cmd

import (
	"strings"
	"testing"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
)

func TestParseSelectedMultiSelectAnswers(t *testing.T) {
	opts := []string{
		"Outlook for Windows, Inc.   Microsoft.Outlook   1.2026.811.200   1.2026.812.100   winget",
		"Git                         Git.Git             2.55.0.2         2.55.0.3         winget",
		"PowerToys                   Microsoft.PowerToys 0.80.0           0.81.0           winget",
	}

	// Test 0 selections
	if res := parseSelectedMultiSelectAnswers("", opts); len(res) != 0 {
		t.Errorf("expected 0 selections for empty answer, got %v", res)
	}

	// Test 1 selection
	res1 := parseSelectedMultiSelectAnswers(opts[0], opts)
	if len(res1) != 1 || res1[0] != opts[0] {
		t.Errorf("test 1 failed: %v", res1)
	}

	// Test 2 selections (including one with a comma in its name)
	ans2 := strings.Join([]string{opts[0], opts[2]}, ", ")
	res2 := parseSelectedMultiSelectAnswers(ans2, opts)
	if len(res2) != 2 || res2[0] != opts[0] || res2[1] != opts[2] {
		t.Errorf("test 2 failed: %v", res2)
	}

	// Test all 3 selections
	ans3 := strings.Join(opts, ", ")
	res3 := parseSelectedMultiSelectAnswers(ans3, opts)
	if len(res3) != 3 || res3[0] != opts[0] || res3[1] != opts[1] || res3[2] != opts[2] {
		t.Errorf("test 3 failed: %v", res3)
	}
}

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
}
