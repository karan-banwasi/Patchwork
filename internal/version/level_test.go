package version

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    BumpLevel
		wantErr bool
	}{
		{"", "", false},
		{"   ", "", false},
		{"patch", LevelPatch, false},
		{"PATCH", LevelPatch, false},
		{"Minor", LevelMinor, false},
		{"  major  ", LevelMajor, false},
		{"invalid", "", true},
		{"prerelease", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAllowsLevel(t *testing.T) {
	tests := []struct {
		max    BumpLevel
		actual BumpLevel
		want   bool
	}{
		{"", LevelPatch, true},
		{"", LevelMinor, true},
		{"", LevelMajor, true},
		{LevelMajor, LevelPatch, true},
		{LevelMajor, LevelMinor, true},
		{LevelMajor, LevelMajor, true},
		{LevelMinor, LevelPatch, true},
		{LevelMinor, LevelMinor, true},
		{LevelMinor, LevelMajor, false},
		{LevelPatch, LevelPatch, true},
		{LevelPatch, LevelMinor, false},
		{LevelPatch, LevelMajor, false},
		{BumpLevel("unknown"), LevelPatch, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.max)+"->"+string(tt.actual), func(t *testing.T) {
			if got := AllowsLevel(tt.max, tt.actual); got != tt.want {
				t.Errorf("AllowsLevel(%q, %q) = %v, want %v", tt.max, tt.actual, got, tt.want)
			}
		})
	}
}

func TestDiffLevel(t *testing.T) {
	tests := []struct {
		name      string
		ecosystem string
		current   string
		available string
		want      BumpLevel
	}{
		// Basic semver
		{"identical", "npm", "1.2.3", "1.2.3", LevelPatch},
		{"patch bump", "npm", "1.2.3", "1.2.4", LevelPatch},
		{"minor bump", "npm", "1.2.3", "1.3.0", LevelMinor},
		{"major bump", "npm", "1.2.3", "2.0.0", LevelMajor},
		{"v prefix", "cargo", "v1.2.3", "v1.3.0", LevelMinor},
		{"two segment minor", "generic", "1.2", "1.3", LevelMinor},
		{"two to three segment patch", "generic", "1.2", "1.2.1", LevelPatch},
		{"four segment dotnet patch", "dotnet", "4.3.0.1", "4.3.0.2", LevelPatch},
		{"build metadata ignored", "cargo", "1.2.3+b1", "1.2.4+b2", LevelPatch},

		// Pre-release and post-release
		{"prerelease to final", "npm", "1.2.3-rc.1", "1.2.3", LevelPatch},
		{"prerelease alpha to beta", "cargo", "1.2.3-alpha.1", "1.2.3-beta.1", LevelPatch},
		{"patch to next major prerelease", "npm", "1.2.3", "2.0.0-rc.1", LevelMajor},
		{"patch to next minor prerelease", "npm", "1.2.3", "1.3.0-beta.1", LevelMinor},
		{"pep440 post release", "uv", "1.0.0", "1.0.0.post1", LevelPatch},

		// Epochs
		{"pep440 epoch increase", "uv", "2.0.0", "1!1.0.0", LevelMajor},
		{"pep440 same epoch minor", "uv", "1!1.0.0", "1!1.1.0", LevelMinor},
		{"apt epoch increase", "apt", "1:3.0.0", "2:1.0.0", LevelMajor},
		{"apt same epoch patch", "apt", "1:2.25.1", "1:2.25.2", LevelPatch},
		{"brew epoch increase", "brew", "1:2.0.0", "2:1.0.0", LevelMajor},

		// Revisions and package releases
		{"brew revision bump", "brew", "1.16.0", "1.16.0_1", LevelPatch},
		{"brew revision to minor", "brew", "1.16.0_2", "1.17.0", LevelMinor},
		{"dnf release bump", "dnf", "7.82.0-15.fc38", "7.82.0-16.fc38", LevelPatch},
		{"pacman minor bump with pkgrel", "pacman", "8.4.0-1", "8.5.0-1", LevelMinor},
		{"zypper major bump with pkgrel", "zypper", "8.4.0-1.1", "9.0.0-1.1", LevelMajor},

		// Sentinels and downgrades
		{"unknown current defaults to major", "generic", "unknown", "1.0.0", LevelMajor},
		{"empty current defaults to major", "generic", "", "1.0.0", LevelMajor},
		{"unknown available", "generic", "1.0.0", "unknown", LevelPatch},
		{"downgrade safe", "generic", "2.0.0", "1.0.0", LevelPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DiffLevel(tt.ecosystem, tt.current, tt.available); got != tt.want {
				t.Errorf("DiffLevel(%q, %q, %q) = %q, want %q", tt.ecosystem, tt.current, tt.available, got, tt.want)
			}
		})
	}
}
