package pm

import (
	"testing"
)

func TestValidators(t *testing.T) {
	tests := []struct {
		name     string
		validate func(string) error
		valid    []string
		invalid  []string
	}{
		{
			name:     "Apt",
			validate: ValidateAptID,
			valid:    []string{"curl", "git", "libssl1.1", "libssl1.1:amd64", "python3-pip"},
			invalid:  []string{"", "-curl", "../etc/passwd", "curl$evil", "CURL", "curl space"},
		},
		{
			name:     "Dnf",
			validate: ValidateDnfID,
			valid:    []string{"curl", "git", "python3.11", "libssl-dev", "Package+Name"},
			invalid:  []string{"", "-curl", "../evil", "pkg$name"},
		},
		{
			name:     "Pacman",
			validate: ValidatePacmanID,
			valid:    []string{"curl", "git", "linux", "linux-headers", "python@3"},
			invalid:  []string{"", "-curl", "../evil", "curl$name"},
		},
		{
			name:     "Zypper",
			validate: ValidateZypperID,
			valid:    []string{"curl", "git", "libopenssl1_1", "zypper-apt"},
			invalid:  []string{"", "-curl", "../evil", "zypper%"},
		},
		{
			name:     "Flatpak",
			validate: ValidateFlatpakID,
			valid:    []string{"org.mozilla.firefox", "com.spotify.Client", "org.videolan.VLC"},
			invalid:  []string{"", "firefox", "-org.mozilla.firefox", "org/mozilla/firefox", "org..firefox"},
		},
		{
			name:     "Snap",
			validate: ValidateSnapID,
			valid:    []string{"firefox", "lxd", "vlc", "core20"},
			invalid:  []string{"", "-snap", "Snap-Upper", "invalid_snap", "../snap"},
		},
		{
			name:     "Mas",
			validate: ValidateMasID,
			valid:    []string{"497799835", "1295203466", "803453959"},
			invalid:  []string{"", "slack", "-123", "123a", "../123"},
		},
		{
			name:     "Pipx",
			validate: ValidatePipxID,
			valid:    []string{"black", "ruff", "flake8", "azure-cli", "my_pkg.tool"},
			invalid:  []string{"", "-black", "../evil", "bad$pkg"},
		},
		{
			name:     "Gem",
			validate: ValidateGemID,
			valid:    []string{"rails", "rubocop", "bundler", "actionpack"},
			invalid:  []string{"", "-rails", "../evil", "rails$"},
		},
		{
			name:     "Rustup",
			validate: ValidateRustupID,
			valid:    []string{"stable", "nightly", "1.74.0", "stable-x86_64-unknown-linux-gnu"},
			invalid:  []string{"", "-stable", "../evil", "rustup%toolchain"},
		},
		{
			name:     "Mise",
			validate: ValidateMiseID,
			valid:    []string{"node", "python", "java@openjdk-21", "ubi:cli/cli"},
			invalid:  []string{"", "-node", "../evil", "node;rm"},
		},
		{
			name:     "Asdf",
			validate: ValidateAsdfID,
			valid:    []string{"nodejs", "python", "golang", "ruby"},
			invalid:  []string{"", "-nodejs", "../evil", "nodejs$"},
		},
		{
			name:     "VSCode",
			validate: ValidateVSCodeID,
			valid:    []string{"golang.go", "ms-python.python", "esbenp.prettier-vscode"},
			invalid:  []string{"", "golang", "-golang.go", "golang..go", "golang.go;rm"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.valid {
				if err := tc.validate(v); err != nil {
					t.Errorf("%s: expected %q to be valid, got: %v", tc.name, v, err)
				}
			}
			for _, inv := range tc.invalid {
				if err := tc.validate(inv); err == nil {
					t.Errorf("%s: expected %q to be invalid, got nil", tc.name, inv)
				}
			}
		})
	}
}
