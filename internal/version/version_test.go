package version

import (
	"testing"
)

func TestCompareAndIsUpgrade(t *testing.T) {
	tests := []struct {
		name        string
		ecosystem   string
		current     string
		available   string
		expectedCmp int
		isUpgrade   bool
	}{
		// Basic numeric
		{"equal basic", "generic", "1.0.0", "1.0.0", 0, false},
		{"simple upgrade", "generic", "1.0.0", "1.0.1", -1, true},
		{"simple downgrade", "generic", "1.0.1", "1.0.0", 1, false},
		{"multi segment upgrade", "generic", "1.2.3.4", "1.2.3.5", -1, true},
		{"leading v prefix", "generic", "v1.2.0", "1.2.1", -1, true},
		{"trailing zero equality", "generic", "1.2", "1.2.0", 0, false},
		{"major bump", "generic", "1.9.9", "2.0.0", -1, true},

		// SemVer pre-releases
		{"semver alpha to beta", "cargo", "1.0.0-alpha.1", "1.0.0-beta.1", -1, true},
		{"semver beta to rc", "npm", "1.0.0-beta.2", "1.0.0-rc.1", -1, true},
		{"semver rc to final", "cargo", "1.0.0-rc.1", "1.0.0", -1, true},
		{"semver final to rc is downgrade", "cargo", "1.0.0", "1.0.0-rc.1", 1, false},
		{"semver rc sequence", "npm", "2.0.0-rc1", "2.0.0-rc2", -1, true},

		// Build metadata
		{"semver build metadata ignored", "cargo", "1.0.0+20130313144700", "1.0.0+20130313144701", 0, false},
		{"semver build metadata with real upgrade", "cargo", "1.0.0+build1", "1.0.1+build2", -1, true},

		// PEP 440
		{"pep440 dev to alpha", "uv", "1.0.0.dev1", "1.0.0a1", -1, true},
		{"pep440 alpha to beta", "python", "1.0.0a1", "1.0.0b1", -1, true},
		{"pep440 beta to rc", "pypi", "1.0.0b1", "1.0.0rc1", -1, true},
		{"pep440 rc to final", "uv", "1.0.0rc1", "1.0.0", -1, true},
		{"pep440 final to post", "python", "1.0.0", "1.0.0.post1", -1, true},
		{"pep440 post to post2", "uv", "1.0.0.post1", "1.0.0.post2", -1, true},
		{"pep440 epoch upgrade", "uv", "2.0.0", "1!1.0.0", -1, true},
		{"pep440 epoch downgrade", "uv", "1!1.0.0", "2.0.0", 1, false},
		{"pep440 same epoch upgrade", "python", "1!1.0.0", "1!1.1.0", -1, true},

		// Homebrew
		{"brew revision bump", "brew", "1.16.0", "1.16.0_1", -1, true},
		{"brew revision sequence", "homebrew", "1.16.0_1", "1.16.0_2", -1, true},
		{"brew core version over revision", "brew", "1.16.0_2", "1.17.0", -1, true},
		{"brew epoch", "brew", "1:2.0", "2:1.0", -1, true},

		// NuGet / .NET / Choco
		{"nuget 4 segment bump", "dotnet", "4.3.0.1", "4.3.0.2", -1, true},
		{"choco identical version", "choco", "2.7.5", "2.7.5", 0, false},
		{"choco downgrade attempt", "choco", "14.51.36247", "14.50.0", 1, false},
		{"psresource preview to release", "psresource", "2.0.0-preview.1", "2.0.0", -1, true},

		// Sentinels
		{"unknown current is upgradeable", "generic", "unknown", "1.0.0", -1, true},
		{"unknown available is not upgrade", "generic", "1.0.0", "unknown", 1, false},
		{"empty current is upgradeable", "generic", "", "1.0.0", -1, true},
		{"empty available is not upgrade", "generic", "1.0.0", "", 1, false},
		{"sentinel casing equality unknown", "generic", "unknown", "Unknown", 0, false},
		{"sentinel casing equality dev", "generic", "dev", "DEV", 0, false},

		// Stable / Final releases over pre-releases
		{"stable over preview", "generic", "2.3.4-preview", "2.3.4-stable", -1, true},
		{"preview under stable", "generic", "2.3.4-stable", "2.3.4-preview", 1, false},
		{"final over beta", "generic", "1.0.0-beta", "1.0.0-final", -1, true},
		{"beta under final", "generic", "1.0.0-final", "1.0.0-beta", 1, false},
		{"release over rc", "generic", "1.0.0-rc.1", "1.0.0-release", -1, true},

		// Candidate pre-releases
		{"candidate to final", "generic", "1.0.0-candidate", "1.0.0", -1, true},
		{"final to candidate is downgrade", "generic", "1.0.0", "1.0.0-candidate", 1, false},
		{"candidate sequence", "generic", "1.0.0-candidate.1", "1.0.0-candidate.2", -1, true},
		{"candidate to final release suffix", "generic", "2.0.0-candidate", "2.0.0-final", -1, true},
		{"beta to candidate", "generic", "1.0.0-beta", "1.0.0-candidate", -1, true},
		{"candidate equal", "generic", "1.0.0-candidate", "1.0.0-candidate", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmp := Compare(tt.ecosystem, tt.current, tt.available)
			if cmp != tt.expectedCmp {
				t.Errorf("Compare(%q, %q, %q) = %d, expected %d", tt.ecosystem, tt.current, tt.available, cmp, tt.expectedCmp)
			}
			isUp := IsUpgrade(tt.ecosystem, tt.current, tt.available)
			if isUp != tt.isUpgrade {
				t.Errorf("IsUpgrade(%q, %q, %q) = %v, expected %v", tt.ecosystem, tt.current, tt.available, isUp, tt.isUpgrade)
			}
		})
	}
}

func TestCandidateUpgrade(t *testing.T) {
	if !IsUpgrade("generic", "1.0.0-candidate", "1.0.0") {
		t.Errorf("FAIL: 1.0.0-candidate should be upgradeable to 1.0.0")
	}
}
