package cmd

import "testing"

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"dev", "v1.0.0", true},
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.5", "v1.1.0", true},
		{"v1.2.0", "v2.0.0", true},
		{"v1.0.5", "v1.0.5", false},
		{"v1.0.6", "v1.0.5", false},
	}

	for _, tc := range tests {
		got := isNewerVersion(tc.current, tc.latest)
		if got != tc.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestExtractExpectedSHA256(t *testing.T) {
	sampleYAML := `PackageIdentifier: KaranBanwasi.Patchwork
PackageVersion: 0.0.22
InstallerType: portable
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw.exe
    InstallerSha256: AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899
  - Architecture: arm64
    InstallerUrl: https://github.com/karan-banwasi/Patchwork/releases/download/v0.0.22/pw-arm64.exe
    InstallerSha256: 11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF
ManifestType: installer
`
	gotX64 := extractExpectedSHA256(sampleYAML, "x64")
	if gotX64 != "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899" {
		t.Errorf("unexpected x64 sha: %q", gotX64)
	}
	gotArm64 := extractExpectedSHA256(sampleYAML, "arm64")
	if gotArm64 != "11223344556677889900AABBCCDDEEFF11223344556677889900AABBCCDDEEFF" {
		t.Errorf("unexpected arm64 sha: %q", gotArm64)
	}
}
