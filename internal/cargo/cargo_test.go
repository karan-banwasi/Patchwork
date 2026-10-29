package cargo

import "testing"

func TestParseCargoInstallList(t *testing.T) {
	sample := `bat v0.24.0:
    bat.exe
ripgrep v14.1.0:
    rg.exe
my-local-tool v0.1.0 (C:\projects\my-local-tool):
    my-local-tool.exe
`
	crates := parseCargoInstallList(sample)
	if len(crates) != 2 {
		t.Fatalf("expected 2 crates.io packages (excluding local path crate), got %d", len(crates))
	}
	if crates[0].Id != "bat" || crates[0].Version != "0.24.0" {
		t.Errorf("unexpected crate[0]: %+v", crates[0])
	}
	if crates[1].Id != "ripgrep" || crates[1].Version != "14.1.0" {
		t.Errorf("unexpected crate[1]: %+v", crates[1])
	}
}
