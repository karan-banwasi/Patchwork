package version

import (
	"testing"
)

func TestAdversarialVersionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		ecosystem   string
		current     string
		available   string
		expectedCmp int
		isUpgrade   bool
	}{
		// Downgrade protection - generic & SemVer
		{"major downgrade", "generic", "2.0.0", "1.0.0", 1, false},
		{"minor downgrade", "generic", "1.5.0", "1.4.9", 1, false},
		{"patch downgrade", "generic", "1.0.10", "1.0.9", 1, false},
		{"subpatch downgrade", "generic", "1.0.0.5", "1.0.0.4", 1, false},
		{"final to rc downgrade", "cargo", "1.0.0", "1.0.0-rc.2", 1, false},
		{"rc to beta downgrade", "cargo", "1.0.0-rc.1", "1.0.0-beta.2", 1, false},
		{"beta to alpha downgrade", "npm", "1.0.0-beta.1", "1.0.0-alpha.5", 1, false},
		{"alpha to dev downgrade", "uv", "1.0.0a1", "1.0.0.dev10", 1, false},
		{"post to final downgrade", "python", "1.0.0.post1", "1.0.0", 1, false},
		{"post2 to post1 downgrade", "uv", "1.0.0.post2", "1.0.0.post1", 1, false},

		// Identical versions (must NEVER be an upgrade)
		{"identical semver", "cargo", "1.2.3", "1.2.3", 0, false},
		{"identical with leading v", "generic", "v1.2.3", "1.2.3", 0, false},
		{"identical with uppercase V", "generic", "V1.2.3", "v1.2.3", 0, false},
		{"identical with trailing zero", "generic", "1.2.0", "1.2", 0, false},
		{"identical 4-part with trailing zeros", "dotnet", "1.0.0.0", "1.0.0", 0, false},
		{"identical rc", "npm", "1.0.0-rc.1", "1.0.0-rc.1", 0, false},
		{"identical pep440 epoch", "uv", "1!2.3.4", "1!2.3.4", 0, false},
		{"identical brew revision", "brew", "1.2.3_2", "1.2.3_2", 0, false},

		// Build metadata (SemVer 2.0.0: build metadata ignored in precedence)
		{"build metadata only diff", "cargo", "1.0.0+20130313144700", "1.0.0+20130313144701", 0, false},
		{"build metadata sha diff", "npm", "1.2.3+sha.123", "1.2.3+sha.456", 0, false},
		{"build metadata vs none", "generic", "1.0.0+build1", "1.0.0", 0, false},
		{"none vs build metadata", "generic", "1.0.0", "1.0.0+build1", 0, false},
		{"build metadata with real upgrade", "cargo", "1.0.0+build1", "1.0.1+build2", -1, true},
		{"build metadata with real downgrade", "cargo", "1.0.1+build1", "1.0.0+build2", 1, false},

		// PEP 440 epochs and phases
		{"pep440 epoch bump dominates version drop", "uv", "999.0.0", "1!0.1.0", -1, true},
		{"pep440 epoch downgrade dominates version bump", "uv", "1!0.1.0", "999.0.0", 1, false},
		{"pep440 epoch 2 vs epoch 1", "python", "1!9.9.9", "2!1.0.0", -1, true},
		{"pep440 epoch 1 vs epoch 2 downgrade", "python", "2!1.0.0", "1!9.9.9", 1, false},
		{"pep440 dev sequence", "uv", "1.0.0.dev1", "1.0.0.dev2", -1, true},
		{"pep440 dev sequence downgrade", "uv", "1.0.0.dev2", "1.0.0.dev1", 1, false},
		{"pep440 post sequence", "pypi", "1.0.0.post1", "1.0.0.post2", -1, true},
		{"pep440 post to next patch", "pypi", "1.0.0.post99", "1.0.1", -1, true},
		{"pep440 rc to post", "uv", "1.0.0rc2", "1.0.0.post1", -1, true},

		// Homebrew revisions and epochs
		{"brew revision bump", "brew", "2.40.0", "2.40.0_1", -1, true},
		{"brew revision bump sequence", "brew", "2.40.0_1", "2.40.0_2", -1, true},
		{"brew revision downgrade", "brew", "2.40.0_2", "2.40.0_1", 1, false},
		{"brew revision to next version ignores revision", "brew", "2.40.0_9", "2.41.0", -1, true},
		{"brew epoch bump", "brew", "1:2.0.0", "2:1.0.0", -1, true},
		{"brew epoch downgrade", "brew", "2:1.0.0", "1:2.0.0", 1, false},
		{"brew epoch same, revision bump", "brew", "1:2.0.0_1", "1:2.0.0_2", -1, true},

		// NuGet / .NET 4-part versions
		{"nuget fourth part bump", "dotnet", "7.0.100.1", "7.0.100.2", -1, true},
		{"nuget fourth part downgrade", "dotnet", "7.0.100.2", "7.0.100.1", 1, false},
		{"nuget preview sequence", "dotnet", "8.0.0-preview.1", "8.0.0-preview.2", -1, true},
		{"nuget preview to rc", "dotnet", "8.0.0-preview.7", "8.0.0-rc.1", -1, true},
		{"nuget rc to rtm", "dotnet", "8.0.0-rc.2", "8.0.0", -1, true},

		// Chocolatey versions
		{"choco downgrade attempt", "choco", "14.51.36247", "14.50.0", 1, false},
		{"choco equal", "choco", "2.7.5", "2.7.5", 0, false},
		{"choco minor bump", "choco", "2.7.5", "2.8.0", -1, true},

		// Whitespace and boundary checks
		{"whitespace padded current", "generic", "  1.2.3  ", "1.2.4", -1, true},
		{"whitespace padded available", "generic", "1.2.3", "  1.2.4  ", -1, true},
		{"whitespace identical", "generic", " 1.2.3 ", "1.2.3", 0, false},
		{"large calendar versions", "generic", "2024.11.01", "2024.11.08", -1, true},
		{"large calendar downgrade", "generic", "2024.11.08", "2024.11.01", 1, false},
		{"multi segment 6 levels", "generic", "1.2.3.4.5.6", "1.2.3.4.5.7", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmp := Compare(tt.ecosystem, tt.current, tt.available)
			if cmp != tt.expectedCmp {
				t.Fatalf("Compare(%q, %q, %q) = %d; want %d", tt.ecosystem, tt.current, tt.available, cmp, tt.expectedCmp)
			}
			isUp := IsUpgrade(tt.ecosystem, tt.current, tt.available)
			if isUp != tt.isUpgrade {
				t.Fatalf("IsUpgrade(%q, %q, %q) = %v; want %v", tt.ecosystem, tt.current, tt.available, isUp, tt.isUpgrade)
			}
		})
	}
}
