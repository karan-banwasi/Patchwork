package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/profile"
)

type installCall struct {
	id      string
	version string
}

type mockProfileManager struct {
	name      string
	aliases   []string
	available bool
	installed []pm.InstalledPackage
	listErr   error
	installFn func(ctx context.Context, id, version string) error
	calls     []installCall
}

func (m *mockProfileManager) Name() string                         { return m.name }
func (m *mockProfileManager) Aliases() []string                    { return m.aliases }
func (m *mockProfileManager) IsAvailable(ctx context.Context) bool { return m.available }
func (m *mockProfileManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *mockProfileManager) UpgradePackage(ctx context.Context, id string) error { return nil }
func (m *mockProfileManager) ValidateID(id string) error                          { return nil }
func (m *mockProfileManager) ListInstalled(ctx context.Context) ([]pm.InstalledPackage, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.installed, nil
}
func (m *mockProfileManager) InstallPackage(ctx context.Context, id, version string) error {
	m.calls = append(m.calls, installCall{id: id, version: version})
	if m.installFn != nil {
		return m.installFn(ctx, id, version)
	}
	return nil
}

func TestExportCmd_PatchfileDSLAndFlags(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", cfgPath)

	cfg := &config.Config{}
	cfg.AddPinEntry(config.PinEntry{Manager: "winget", ID: "Git.Git", Version: "<3.0", Reason: "stay on v2"})
	cfg.AddPinEntry(config.PinEntry{Manager: "npm", ID: "eslint"})
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	wgMgr := &mockProfileManager{
		name:      "winget",
		available: true,
		installed: []pm.InstalledPackage{
			{Name: "Git", ID: "Git.Git", Version: "2.45.0", ManagerName: "winget"},
			{Name: "Node", ID: "OpenJS.NodeJS", Version: "20.12.0", ManagerName: "winget"},
		},
	}
	npmMgr := &mockProfileManager{
		name:      "npm",
		available: true,
		installed: []pm.InstalledPackage{
			{Name: "typescript", ID: "typescript", Version: "5.4.5", ManagerName: "npm"},
		},
	}

	registryOverride = pm.NewRegistry(wgMgr, npmMgr)
	t.Cleanup(func() { registryOverride = nil })

	// Default export (no versions, with pins)
	var stdout, stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"export"})
	if err := root.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, `winget "Git.Git"`) || !strings.Contains(outStr, `npm "typescript"`) {
		t.Errorf("expected packages in output:\n%s", outStr)
	}
	if strings.Contains(outStr, `version="20.12.0"`) {
		t.Errorf("did not expect package versions without --include-versions:\n%s", outStr)
	}
	if !strings.Contains(outStr, `pin "winget:Git.Git" version="<3.0" reason="stay on v2"`) {
		t.Errorf("expected winget pin in output:\n%s", outStr)
	}

	// --include-versions --include-pins=false -m winget -o <file>
	outFile := filepath.Join(t.TempDir(), "Patchfile")
	stdout.Reset()
	root2 := NewRootCmd()
	root2.SetOut(&stdout)
	root2.SetErr(&stderr)
	root2.SetArgs([]string{"export", "-m", "winget", "--include-versions", "--include-pins=false", "-o", outFile})
	if err := root2.Execute(); err != nil {
		t.Fatalf("export -o failed: %v", err)
	}

	fileBytes, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed reading exported file: %v", err)
	}
	parsed, err := profile.ParsePatchfile(fileBytes)
	if err != nil {
		t.Fatalf("failed parsing exported file: %v", err)
	}
	if len(parsed.Packages) != 2 || parsed.Packages[0].Version != "2.45.0" || len(parsed.Pins) != 0 {
		t.Errorf("unexpected exported profile: %+v", parsed)
	}
}

func TestExportCmd_JSONAndPartialWarning(t *testing.T) {
	t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))

	okMgr := &mockProfileManager{
		name:      "npm",
		available: true,
		installed: []pm.InstalledPackage{{Name: "tsx", ID: "tsx", Version: "4.15.0", ManagerName: "npm"}},
	}
	failMgr := &mockProfileManager{
		name:      "bun",
		available: true,
		listErr:   errors.New("bun failed"),
	}

	registryOverride = pm.NewRegistry(okMgr, failMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"export", "--json", "--include-versions"})
	if err := root.Execute(); err != nil {
		t.Fatalf("export --json failed: %v", err)
	}

	if !strings.Contains(stderr.String(), "Warning: [bun] bun failed") {
		t.Errorf("expected partial failure warning on stderr, got: %q", stderr.String())
	}

	var p profile.Profile
	if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if p.SchemaVersion != "1.0.0" || len(p.Packages) != 1 || p.Packages[0].ID != "tsx" || p.Packages[0].Version != "4.15.0" {
		t.Errorf("unexpected JSON profile: %+v", p)
	}
}

func TestImportCmd_WorkflowAndExitCodes(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", cfgPath)

	wgMgr := &mockProfileManager{
		name:      "winget",
		available: true,
		installed: []pm.InstalledPackage{
			{Name: "Git", ID: "Git.Git", Version: "2.45.0", ManagerName: "winget"},
		},
	}
	npmMgr := &mockProfileManager{
		name:      "npm",
		available: true,
		installed: []pm.InstalledPackage{},
	}

	registryOverride = pm.NewRegistry(wgMgr, npmMgr)
	t.Cleanup(func() { registryOverride = nil })

	dsl := `# machine profile
winget "Git.Git"
winget "OpenJS.NodeJS" version="20.12.0"
npm "typescript" version="5.4.5"
pin "winget:Git.Git" version="<3.0" reason="stay on v2"
`
	tmpDir := t.TempDir()
	patchPath := filepath.Join(tmpDir, "Patchfile")
	if err := os.WriteFile(patchPath, []byte(dsl), 0644); err != nil {
		t.Fatalf("failed writing Patchfile: %v", err)
	}

	// 1. Dry-run from explicit file
	var stdout, stderr bytes.Buffer
	rootDry := NewRootCmd()
	rootDry.SetOut(&stdout)
	rootDry.SetErr(&stderr)
	rootDry.SetArgs([]string{"import", patchPath, "--dry-run", "--json"})
	if err := rootDry.Execute(); err != nil {
		t.Fatalf("import --dry-run failed: %v", err)
	}
	if len(wgMgr.calls) != 0 || len(npmMgr.calls) != 0 {
		t.Fatalf("expected 0 install calls in dry-run, got winget=%v npm=%v", wgMgr.calls, npmMgr.calls)
	}
	var dryOut importJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &dryOut); err != nil {
		t.Fatalf("invalid dry-run JSON: %v", err)
	}
	if !dryOut.DryRun || dryOut.Summary.Skipped != 1 || dryOut.Summary.DryRun != 2 || dryOut.PinsRestored != 1 {
		t.Errorf("unexpected dry-run JSON summary: %+v", dryOut)
	}

	// 2. Real import from stdin ('-') with --skip-existing=true
	stdout.Reset()
	rootReal := NewRootCmd()
	rootReal.SetIn(strings.NewReader(dsl))
	rootReal.SetOut(&stdout)
	rootReal.SetErr(&stderr)
	rootReal.SetArgs([]string{"import", "-", "--json"})
	if err := rootReal.Execute(); err != nil {
		t.Fatalf("import from stdin failed: %v", err)
	}
	if len(wgMgr.calls) != 1 || wgMgr.calls[0].id != "OpenJS.NodeJS" || wgMgr.calls[0].version != "20.12.0" {
		t.Errorf("expected winget install for OpenJS.NodeJS@20.12.0 only, got %+v", wgMgr.calls)
	}
	if len(npmMgr.calls) != 1 || npmMgr.calls[0].id != "typescript" || npmMgr.calls[0].version != "5.4.5" {
		t.Errorf("expected npm install for typescript@5.4.5, got %+v", npmMgr.calls)
	}

	loadedCfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("failed loading restored config: %v", err)
	}
	if len(loadedCfg.Pins) != 1 || loadedCfg.Pins[0].ID != "Git.Git" || loadedCfg.Pins[0].Version != "<3.0" {
		t.Errorf("expected pin to be restored into config, got %+v", loadedCfg.Pins)
	}

	// 3. Failure returns ExitUpgradeFailure (2)
	wgMgr.calls = nil
	npmMgr.calls = nil
	npmMgr.installFn = func(ctx context.Context, id, version string) error {
		return errors.New("network failure")
	}

	stdout.Reset()
	rootFail := NewRootCmd()
	rootFail.SetOut(&stdout)
	rootFail.SetErr(&stderr)
	rootFail.SetArgs([]string{"import", patchPath, "-m", "npm", "--skip-existing=false"})
	err = rootFail.Execute()
	if err == nil {
		t.Fatal("expected error when install fails")
	}
	if code := exitcodes.Determine(context.Background(), err); code != exitcodes.ExitUpgradeFailure {
		t.Errorf("exit code = %d, want %d (err=%v)", code, exitcodes.ExitUpgradeFailure, err)
	}
	if len(wgMgr.calls) != 0 {
		t.Errorf("expected -m npm to filter out winget packages, got %+v", wgMgr.calls)
	}
}

func TestImportCmd_DefaultPatchfileInCwd(t *testing.T) {
	t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	npmMgr := &mockProfileManager{
		name:      "npm",
		available: true,
	}
	registryOverride = pm.NewRegistry(npmMgr)
	t.Cleanup(func() { registryOverride = nil })

	workDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	if err := os.WriteFile(filepath.Join(workDir, "Patchfile"), []byte("npm \"tsx\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"import"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import default Patchfile failed: %v", err)
	}
	if len(npmMgr.calls) != 1 || npmMgr.calls[0].id != "tsx" {
		t.Errorf("expected tsx install, got %+v", npmMgr.calls)
	}
	if !strings.Contains(stdout.String(), "Installed: 1") {
		t.Errorf("expected summary table with Installed: 1, got:\n%s", stdout.String())
	}
}
