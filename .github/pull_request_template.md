## Description
<!-- Provide a clear, concise summary of the changes introduced in this pull request and the rationale behind them. -->

## Related Issues
<!-- Reference any related issues here. Format: Fixes #123, Closes #456, or Addresses #789 -->
Fixes #

## Type of Change
<!-- Please check the options that apply: -->
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Architectural refactoring (no user-facing behavioral changes)
- [ ] Documentation update
- [ ] CI / Build / Dependencies

## Testing & Cross-Platform Verification
<!-- Detail the test steps and platforms you used to verify these changes. -->
- **Operating Systems Tested**:
  - [ ] Windows
  - [ ] macOS
  - [ ] Linux
- **Package Managers Tested** (if applicable):
  - [ ] Winget / Scoop / Chocolatey
  - [ ] Homebrew
  - [ ] npm / uv / cargo / dotnet / psresource

## Pre-Submission Checklist
<!-- Please ensure all items below are checked before submitting your PR: -->
- [ ] My code adheres to the project's coding style guidelines (`gofmt -s -w .`).
- [ ] I have verified that no direct `fmt.Print*` or `os.Stdout`/`os.Stderr` calls exist in `internal/` packages (verified via `grep -rnE 'fmt\.Print|os\.Stdout|os\.Stderr' internal/ | grep -v '_test\.go:'`).
- [ ] I have run `go test -count=1 ./...` and all tests pass locally.
- [ ] I have run `go vet ./...` and resolved any issues.
- [ ] I have added or updated tests covering my changes (including golden parser fixtures if modifying package manager parsers).
- [ ] I have updated corresponding documentation (`README.md`, `ARCHITECTURE.md`, `docs/`) if applicable.
- [ ] Platform-specific code is properly isolated behind build tags (`//go:build ...`).
- [ ] I understand that the CI verification invariant requires all remote GitHub Actions matrix jobs (`ubuntu-latest`, `macos-latest`, `windows-latest`) to finish with status `completed` and conclusion `success` before this PR can be merged.
