package version

import (
	"testing"
)

func TestChallenger_CandidateExhaustive(t *testing.T) {
	cases := []struct {
		name      string
		ecosystem string
		current   string
		available string
		wantCmp   int
		wantUp    bool
	}{
		// Basic candidate vs final
		{"candidate to final", "generic", "1.0.0-candidate", "1.0.0", -1, true},
		{"final to candidate", "generic", "1.0.0", "1.0.0-candidate", 1, false},
		{"candidate equal", "generic", "1.0.0-candidate", "1.0.0-candidate", 0, false},

		// Casing
		{"uppercase candidate", "generic", "1.0.0-CANDIDATE", "1.0.0", -1, true},
		{"mixed case candidate", "generic", "1.0.0-Candidate.1", "1.0.0-candidate.2", -1, true},

		// Numeric tags
		{"candidate numeric unseparated", "generic", "1.0.0-candidate1", "1.0.0-candidate2", -1, true},
		{"candidate numeric dot-separated", "generic", "1.0.0-candidate.1", "1.0.0-candidate.2", -1, true},
		{"candidate numeric reverse", "generic", "1.0.0-candidate2", "1.0.0-candidate1", 1, false},

		// Pre-release hierarchy with candidate
		{"dev to candidate", "generic", "1.0.0-dev", "1.0.0-candidate", -1, true},
		{"alpha to candidate", "generic", "1.0.0-alpha", "1.0.0-candidate", -1, true},
		{"beta to candidate", "generic", "1.0.0-beta", "1.0.0-candidate", -1, true},
		{"preview to candidate", "generic", "1.0.0-preview", "1.0.0-candidate", -1, true},
		{"candidate to post", "generic", "1.0.0-candidate", "1.0.0-post1", -1, true},

		// Candidate vs RC equivalence
		{"candidate and rc equal phase", "generic", "1.0.0-candidate", "1.0.0-rc", 0, false},
		{"candidate.1 and rc.1 equal phase", "generic", "1.0.0-candidate.1", "1.0.0-rc.1", 0, false},
		{"candidate.1 to rc.2", "generic", "1.0.0-candidate.1", "1.0.0-rc.2", -1, true},
		{"rc.1 to candidate.2", "generic", "1.0.0-rc.1", "1.0.0-candidate.2", -1, true},

		// Suffixes
		{"candidate to final suffix", "generic", "1.0.0-candidate", "1.0.0-final", -1, true},
		{"candidate to release suffix", "generic", "1.0.0-candidate", "1.0.0-release", -1, true},
		{"candidate to stable suffix", "generic", "1.0.0-candidate", "1.0.0-stable", -1, true},

		// Version jumps
		{"older minor candidate to newer candidate", "generic", "1.0.0-candidate", "1.1.0-candidate", -1, true},
		{"newer major candidate to older final", "generic", "2.0.0-candidate", "1.9.9", 1, false},
		{"older final to newer major candidate", "generic", "1.9.9", "2.0.0-candidate", -1, true},

		// Ecosystems
		{"brew with candidate", "brew", "1.0.0-candidate", "1.0.0", -1, true},
		{"choco with candidate", "choco", "1.0.0-candidate", "1.0.0", -1, true},
		{"npm with candidate", "npm", "1.0.0-candidate", "1.0.0", -1, true},
		{"cargo with candidate", "cargo", "1.0.0-candidate", "1.0.0", -1, true},
		{"dotnet with candidate", "dotnet", "1.0.0-candidate", "1.0.0", -1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCmp := Compare(tc.ecosystem, tc.current, tc.available)
			if gotCmp != tc.wantCmp {
				t.Errorf("Compare(%s, %q, %q) = %d, want %d", tc.ecosystem, tc.current, tc.available, gotCmp, tc.wantCmp)
			}
			gotUp := IsUpgrade(tc.ecosystem, tc.current, tc.available)
			if gotUp != tc.wantUp {
				t.Errorf("IsUpgrade(%s, %q, %q) = %v, want %v", tc.ecosystem, tc.current, tc.available, gotUp, tc.wantUp)
			}
		})
	}
}
