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
		{"pep440 pipx post upgrade", "pipx", "23.9.0", "23.9.1", -1, true},

		// Linux / Epoch aware
		{"apt epoch upgrade", "apt", "1:2.25.1", "1:2.25.2", -1, true},
		{"apt higher epoch beats higher core", "apt", "1:3.0.0", "2:1.0.0", -1, true},
		{"dnf version upgrade", "dnf", "7.82.0-15.fc38", "7.82.0-16.fc38", -1, true},
		{"pacman arch upgrade", "pacman", "8.4.0-1", "8.5.0-1", -1, true},
		{"zypper version upgrade", "zypper", "8.4.0-1.1", "8.5.0-1.1", -1, true},

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

func TestEvaluateConstraint(t *testing.T) {
	tests := []struct {
		name       string
		eco        string
		ver        string
		constraint string
		expected   bool
		wantErr    bool
	}{
		{"empty constraint blocks", "generic", "1.0.0", "", false, false},
		{"empty version blocks", "generic", "", "<23", false, false},
		{"exact match true", "generic", "22.1.0", "22.1.0", true, false},
		{"exact match false", "generic", "22.1.1", "22.1.0", false, false},
		{"equal operator true", "generic", "22.1.0", "=22.1.0", true, false},
		{"double equal true", "generic", "22.1.0", "==22.1.0", true, false},
		{"not equal true", "generic", "22.1.1", "!=22.1.0", true, false},
		{"not equal false", "generic", "22.1.0", "!=22.1.0", false, false},
		{"less than true", "generic", "22.5.0", "<23", true, false},
		{"less than false boundary", "generic", "23.0.0", "<23", false, false},
		{"less than false higher", "generic", "24.1.0", "<23", false, false},
		{"less than or equal boundary", "generic", "22.5.0", "<=22.5.0", true, false},
		{"less than or equal lower", "generic", "22.4.9", "<=22.5.0", true, false},
		{"less than or equal higher", "generic", "22.5.1", "<=22.5.0", false, false},
		{"greater than true", "generic", "1.0.1", ">1.0.0", true, false},
		{"greater than false boundary", "generic", "1.0.0", ">1.0.0", false, false},
		{"greater than false lower", "generic", "0.9.9", ">1.0.0", false, false},
		{"greater than or equal boundary", "generic", "1.0.0", ">=1.0.0", true, false},
		{"greater than or equal higher", "generic", "1.0.1", ">=1.0.0", true, false},
		{"greater than or equal lower", "generic", "0.9.9", ">=1.0.0", false, false},
		{"tilde patch allow", "generic", "1.2.5", "~1.2.3", true, false},
		{"tilde patch disallow minor bump", "generic", "1.3.0", "~1.2.3", false, false},
		{"tilde patch disallow lower", "generic", "1.2.2", "~1.2.3", false, false},
		{"tilde minor format allow", "generic", "1.2.9", "~1.2", true, false},
		{"tilde minor format disallow next minor", "generic", "1.3.0", "~1.2", false, false},
		{"caret minor allow", "generic", "1.3.5", "^1.2.0", true, false},
		{"caret disallow next major", "generic", "2.0.0", "^1.2.0", false, false},
		{"caret disallow lower", "generic", "1.1.9", "^1.2.0", false, false},
		{"caret zero major allow patch", "generic", "0.2.5", "^0.2.1", true, false},
		{"caret zero major disallow minor bump", "generic", "0.3.0", "^0.2.1", false, false},
		{"compound range allow", "generic", "21.5.0", ">=20.0, <23.0", true, false},
		{"compound range disallow lower", "generic", "19.9.0", ">=20.0, <23.0", false, false},
		{"compound range disallow upper", "generic", "23.0.0", ">=20.0, <23.0", false, false},
		{"invalid constraint missing operand", "generic", "1.0.0", "<", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateConstraint(tt.eco, tt.ver, tt.constraint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EvaluateConstraint(%q, %q, %q) unexpected error: %v", tt.eco, tt.ver, tt.constraint, err)
			}
			if got != tt.expected {
				t.Errorf("EvaluateConstraint(%q, %q, %q) = %v, expected %v", tt.eco, tt.ver, tt.constraint, got, tt.expected)
			}
		})
	}
}
