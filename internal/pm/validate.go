package pm

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	genericIDRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+@/\-]{0,255}$`)
	wingetIDRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_+]{0,255}$`)
	scoopIDRe      = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_\-.]*/)?[A-Za-z0-9][A-Za-z0-9_\-.+]{0,255}$`)
	chocoIDRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_]{0,255}$`)
	npmIDRe        = regexp.MustCompile(`^(@[a-z0-9~][a-z0-9\-._~]*/)?[a-z0-9~][a-z0-9\-._~]{0,213}$`)
	cargoIDRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_\-]{0,63}$`)
	uvIDRe         = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._\-]{0,253}[A-Za-z0-9])?$`)
	dotnetIDRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,255}$`)
	psresourceIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,255}$`)
)

func checkCommonID(id string) error {
	if id == "" {
		return fmt.Errorf("invalid package id %q: empty identifier", id)
	}
	if strings.HasPrefix(id, "-") {
		return fmt.Errorf("invalid package id %q: must not start with '-'", id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("invalid package id %q: path traversal sequence not allowed", id)
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("invalid package id %q: contains control character", id)
		}
		// Reject Unicode single/double quotation marks treated by PowerShell as string delimiters
		if r >= 0x2018 && r <= 0x201F {
			return fmt.Errorf("invalid package id %q: contains quote character", id)
		}
	}
	return nil
}

func validateWithPattern(id string, re *regexp.Regexp) error {
	if err := checkCommonID(id); err != nil {
		return err
	}
	if !re.MatchString(id) {
		return fmt.Errorf("invalid package id %q", id)
	}
	return nil
}

// ValidateGenericID validates a package identifier against a conservative cross-manager pattern.
func ValidateGenericID(id string) error {
	return validateWithPattern(id, genericIDRe)
}

// ValidateWingetID validates a winget package identifier.
func ValidateWingetID(id string) error {
	return validateWithPattern(id, wingetIDRe)
}

// ValidateScoopID validates a Scoop package identifier (optionally prefixed by bucket/).
func ValidateScoopID(id string) error {
	return validateWithPattern(id, scoopIDRe)
}

// ValidateChocoID validates a Chocolatey package identifier.
func ValidateChocoID(id string) error {
	return validateWithPattern(id, chocoIDRe)
}

// ValidateNpmID validates an npm package identifier (including @scope/pkg).
func ValidateNpmID(id string) error {
	return validateWithPattern(id, npmIDRe)
}

// ValidateCargoID validates a Rust crate identifier.
func ValidateCargoID(id string) error {
	return validateWithPattern(id, cargoIDRe)
}

// ValidateUvID validates a PyPI / uv tool identifier.
func ValidateUvID(id string) error {
	return validateWithPattern(id, uvIDRe)
}

// ValidateDotnetID validates a .NET global tool NuGet package identifier.
func ValidateDotnetID(id string) error {
	return validateWithPattern(id, dotnetIDRe)
}

// ValidatePSResourceID validates a PowerShell PSResource module identifier.
func ValidatePSResourceID(id string) error {
	return validateWithPattern(id, psresourceIDRe)
}

// EscapeWindowsArg escapes a command-line argument using Windows CommandLineToArgvW rules
// (identical to golang.org/x/sys/windows.EscapeArg, portable across all build platforms).
func EscapeWindowsArg(s string) string {
	if len(s) == 0 {
		return `""`
	}
	needsQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', ' ', '\t':
			needsQuote = true
		}
	}
	if !needsQuote {
		return s
	}
	return QuoteWindowsArg(s)
}

// QuoteWindowsArg always wraps s in double quotes and escapes internal quotes and backslashes
// according to Windows CommandLineToArgvW rules.
func QuoteWindowsArg(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b.WriteByte('\\')
			}
			b.WriteString(`\"`)
		default:
			slashes = 0
		}
		if c != '"' {
			b.WriteByte(c)
		}
	}
	for ; slashes > 0; slashes-- {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}

// EscapeBatchText escapes cmd.exe metacharacters so a string can be safely used in echo or title lines.
func EscapeBatchText(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\r', '\n':
			continue
		case '%':
			b.WriteString("%%")
		case '&', '|', '<', '>', '^', '(', ')', '!', '"':
			b.WriteByte('^')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
