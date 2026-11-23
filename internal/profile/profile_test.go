package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
)

func TestFormatAndParsePatchfile_RoundTrip(t *testing.T) {
	exp := time.Date(2027, 6, 15, 12, 30, 0, 0, time.UTC)
	orig := Profile{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Date(2026, 11, 17, 4, 10, 22, 0, time.UTC),
		Packages: []ProfilePackage{
			{Manager: "winget", ID: "Git.Git"},
			{Manager: "winget", ID: "OpenJS.NodeJS", Version: "20.12.0"},
			{Manager: "npm", ID: "@scope/pkg", Version: "1.2.3"},
		},
		Pins: []config.PinEntry{
			{Manager: "winget", ID: "Git.Git", Version: "<3.0", Reason: `stay on "v2" # LTS`, ExpiresAt: &exp},
			{ID: "ripgrep", Reason: "manual hold"},
		},
	}

	dsl := FormatPatchfile(orig)
	if !strings.Contains(dsl, `winget "Git.Git"`) {
		t.Fatalf("expected winget line in DSL, got:\n%s", dsl)
	}
	if !strings.Contains(dsl, `winget "OpenJS.NodeJS" version="20.12.0"`) {
		t.Fatalf("expected versioned winget line in DSL, got:\n%s", dsl)
	}
	if !strings.Contains(dsl, `pin "winget:Git.Git" version="<3.0"`) {
		t.Fatalf("expected pin line in DSL, got:\n%s", dsl)
	}

	parsed, err := ParsePatchfile([]byte(dsl))
	if err != nil {
		t.Fatalf("ParsePatchfile failed: %v", err)
	}

	if parsed.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", parsed.SchemaVersion, SchemaVersion)
	}
	if len(parsed.Packages) != len(orig.Packages) {
		t.Fatalf("len(Packages) = %d, want %d", len(parsed.Packages), len(orig.Packages))
	}
	for i, got := range parsed.Packages {
		want := orig.Packages[i]
		if got.Manager != want.Manager || got.ID != want.ID || got.Version != want.Version {
			t.Errorf("package[%d] = %+v, want %+v", i, got, want)
		}
	}

	if len(parsed.Pins) != len(orig.Pins) {
		t.Fatalf("len(Pins) = %d, want %d", len(parsed.Pins), len(orig.Pins))
	}
	if parsed.Pins[0].Manager != "winget" || parsed.Pins[0].ID != "Git.Git" || parsed.Pins[0].Version != "<3.0" || parsed.Pins[0].Reason != `stay on "v2" # LTS` {
		t.Errorf("pin[0] = %+v", parsed.Pins[0])
	}
	if parsed.Pins[0].ExpiresAt == nil || !parsed.Pins[0].ExpiresAt.Equal(exp) {
		t.Errorf("pin[0].ExpiresAt = %v, want %v", parsed.Pins[0].ExpiresAt, exp)
	}
	if parsed.Pins[1].Manager != "" || parsed.Pins[1].ID != "ripgrep" || parsed.Pins[1].Reason != "manual hold" {
		t.Errorf("pin[1] = %+v", parsed.Pins[1])
	}
}

func TestParsePatchfile_JSON(t *testing.T) {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	orig := Profile{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Date(2026, 11, 17, 4, 10, 22, 0, time.UTC),
		Packages: []ProfilePackage{
			{Manager: "Brew", ID: "jq", Name: "jq", Version: "1.7.1"},
		},
		Pins: []config.PinEntry{
			{ID: "brew:jq", Version: "<2.0", Reason: "stay on 1.x", ExpiresAt: &exp},
		},
	}

	raw, err := MarshalJSON(orig)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if strings.Contains(string(raw), `\u003c`) {
		t.Errorf("expected unescaped '<' in JSON output, got:\n%s", string(raw))
	}

	parsed, err := ParsePatchfile([]byte("  \n" + string(raw)))
	if err != nil {
		t.Fatalf("ParsePatchfile JSON failed: %v", err)
	}
	if len(parsed.Packages) != 1 || parsed.Packages[0].Manager != "brew" || parsed.Packages[0].ID != "jq" {
		t.Fatalf("unexpected parsed packages: %+v", parsed.Packages)
	}
	if len(parsed.Pins) != 1 || parsed.Pins[0].Manager != "brew" || parsed.Pins[0].ID != "jq" {
		t.Fatalf("unexpected parsed pins: %+v", parsed.Pins)
	}
}

func TestParsePatchfile_CommentsAndInlineComments(t *testing.T) {
	input := `
# Leading comment

winget "Git.Git" # inline comment
npm "typescript" version="5.4.5"
pin "npm:typescript" expires="2027-12-31"
`
	p, err := ParsePatchfile([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(p.Packages))
	}
	if p.Packages[0].ID != "Git.Git" || p.Packages[1].Version != "5.4.5" {
		t.Errorf("unexpected packages: %+v", p.Packages)
	}
	if len(p.Pins) != 1 || p.Pins[0].ExpiresAt == nil {
		t.Errorf("expected pin with parsed YYYY-MM-DD expiry, got %+v", p.Pins)
	}
}

func TestParsePatchfile_Errors(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing id", "winget\n"},
		{"unterminated quote", "winget \"Git.Git\n"},
		{"unknown pkg attr", "winget \"Git.Git\" foo=\"bar\"\n"},
		{"unknown pin attr", "pin \"Git.Git\" foo=\"bar\"\n"},
		{"missing equals", "winget \"Git.Git\" version\n"},
		{"invalid expires", "pin \"Git.Git\" expires=\"not-a-date\"\n"},
		{"bad json", "{not json"},
		{"wrong schema version", `{"schema_version":"9.9.9","packages":[]}`},
		{"missing pkg manager in json", `{"schema_version":"1.0.0","packages":[{"id":"git"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParsePatchfile([]byte(tc.input)); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "Patchfile")
	content := []byte("npm \"typescript\"\n")
	if err := WriteAtomic(target, content, 0644); err != nil {
		t.Fatalf("WriteAtomic failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", string(got), string(content))
	}
}
