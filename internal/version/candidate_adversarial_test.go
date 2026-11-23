package version

import (
	"testing"
)

func TestAdversarialCandidateVersionParsing(t *testing.T) {
	cases := []struct {
		name        string
		ecosystem   string
		v1          string
		v2          string
		expectedCmp int
		isUpgrade   bool
	}{
		// Dispatch requirements
		{"candidate vs final", "generic", "1.0.0-candidate", "1.0.0", -1, true},
		{"candidate.1 vs candidate.2", "generic", "1.0.0-candidate.1", "1.0.0-candidate.2", -1, true},
		{"rc.1 vs candidate.1 equivalent rank", "generic", "1.0.0-rc.1", "1.0.0-candidate.1", 0, false},
		{"candidate vs final release suffix", "generic", "2.0.0-candidate", "2.0.0-final", -1, true},

		// Cross-comparisons between RC and Candidate
		{"candidate.1 to rc.2", "generic", "1.0.0-candidate.1", "1.0.0-rc.2", -1, true},
		{"rc.1 to candidate.2", "generic", "1.0.0-rc.1", "1.0.0-candidate.2", -1, true},
		{"candidate.2 to rc.1 downgrade", "generic", "1.0.0-candidate.2", "1.0.0-rc.1", 1, false},

		// Delimiter and naming variations
		{"dash vs dot candidate", "generic", "1.0.0.candidate.1", "1.0.0.candidate.2", -1, true},
		{"underscore candidate", "generic", "1.0.0_candidate_1", "1.0.0_candidate_2", -1, true},
		{"candidate without number to candidate.1", "generic", "1.0.0-candidate", "1.0.0-candidate.1", -1, true},
		{"case insensitive candidate", "generic", "1.0.0-CANDIDATE.1", "1.0.0-candidate.2", -1, true},
		{"candidate to higher patch", "generic", "1.0.0-candidate", "1.0.1-candidate", -1, true},
		{"candidate to higher minor", "generic", "1.0.0-candidate", "1.1.0-candidate", -1, true},
		{"candidate to higher major", "generic", "1.0.0-candidate", "2.0.0-candidate", -1, true},

		// Hierarchy check: alpha < beta < preview < rc/candidate < final < post
		{"alpha to candidate", "generic", "1.0.0-alpha.5", "1.0.0-candidate.1", -1, true},
		{"beta to candidate", "generic", "1.0.0-beta.9", "1.0.0-candidate.1", -1, true},
		{"preview to candidate", "generic", "1.0.0-preview.2", "1.0.0-candidate.1", -1, true},
		{"candidate to post", "generic", "1.0.0-candidate.1", "1.0.0-post1", -1, true},

		// Across ecosystems
		{"choco candidate", "choco", "3.0.0-candidate", "3.0.0", -1, true},
		{"uv candidate", "uv", "1.2.0-candidate.1", "1.2.0", -1, true},
		{"npm candidate", "npm", "4.5.6-candidate.1", "4.5.6-candidate.2", -1, true},
		{"cargo candidate", "cargo", "0.9.0-candidate", "0.9.0", -1, true},
		{"brew candidate", "brew", "1.0.0-candidate", "1.0.0", -1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmp := Compare(tc.ecosystem, tc.v1, tc.v2)
			if cmp != tc.expectedCmp {
				t.Errorf("[%s] Compare(%q, %q) = %d; want %d", tc.ecosystem, tc.v1, tc.v2, cmp, tc.expectedCmp)
			}
			isUp := IsUpgrade(tc.ecosystem, tc.v1, tc.v2)
			if isUp != tc.isUpgrade {
				t.Errorf("[%s] IsUpgrade(%q, %q) = %v; want %v", tc.ecosystem, tc.v1, tc.v2, isUp, tc.isUpgrade)
			}
		})
	}
}
