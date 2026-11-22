package pm

import (
	"strings"
	"testing"
)

func FuzzValidateIDs(f *testing.F) {
	seeds := []string{
		"Git.Git",
		"@scope/pkg",
		"main/jq",
		"ripgrep",
		"123456789",
		"../evil",
		"-flag",
		"pkg;rm -rf /",
		"pkg\x00null",
		"pkg\u2018quote",
		"\x1b[31mred\x1b[0m",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	validators := []func(string) error{
		ValidateGenericID,
		ValidateWingetID,
		ValidateScoopID,
		ValidateChocoID,
		ValidateNpmID,
		ValidateCargoID,
		ValidateUvID,
		ValidateDotnetID,
		ValidateBrewID,
		ValidatePSResourceID,
		ValidateAptID,
		ValidateDnfID,
		ValidatePacmanID,
		ValidateZypperID,
		ValidateFlatpakID,
		ValidateSnapID,
		ValidateMasID,
		ValidatePipxID,
		ValidateGemID,
		ValidateRustupID,
		ValidateMiseID,
		ValidateAsdfID,
		ValidateVSCodeID,
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1024 {
			return
		}

		for i, validate := range validators {
			if err := validate(input); err == nil {
				if input == "" {
					t.Fatalf("validator[%d] accepted empty identifier", i)
				}
				if strings.HasPrefix(input, "-") {
					t.Fatalf("validator[%d] accepted leading dash identifier %q", i, input)
				}
				if strings.Contains(input, "..") {
					t.Fatalf("validator[%d] accepted path traversal identifier %q", i, input)
				}
				if strings.ContainsAny(input, "\x00\r\n;&|`$<>") {
					t.Fatalf("validator[%d] accepted unsafe shell characters in %q", i, input)
				}
			}
		}

		_ = ValidateVersion(input)
		_ = ValidateOptionalVersion(input)
		_ = EscapeWindowsArg(input)
		_ = QuoteWindowsArg(input)
		_ = EscapeBatchText(input)
		_ = StripANSI(input)
	})
}
