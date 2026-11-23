package version

import (
	"fmt"
	"strings"
)

// BumpLevel represents the semantic version bump magnitude between two versions.
type BumpLevel string

const (
	LevelPatch BumpLevel = "patch"
	LevelMinor BumpLevel = "minor"
	LevelMajor BumpLevel = "major"
)

// ParseLevel parses a user-supplied bump level string case-insensitively.
// An empty string is allowed and returns "" with nil error (representing no ceiling).
func ParseLevel(s string) (BumpLevel, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	switch BumpLevel(norm) {
	case "":
		return "", nil
	case LevelPatch:
		return LevelPatch, nil
	case LevelMinor:
		return LevelMinor, nil
	case LevelMajor:
		return LevelMajor, nil
	default:
		return "", fmt.Errorf("invalid bump level %q (expected patch, minor, or major)", s)
	}
}

// AllowsLevel reports whether actual is permitted under the maxLevel ceiling.
// Ceiling semantics:
//   - "patch": allows patch only
//   - "minor": allows patch and minor
//   - "major" or "": allows all levels (patch, minor, major)
func AllowsLevel(maxLevel, actual BumpLevel) bool {
	maxNorm := BumpLevel(strings.ToLower(strings.TrimSpace(string(maxLevel))))
	actNorm := BumpLevel(strings.ToLower(strings.TrimSpace(string(actual))))

	switch maxNorm {
	case "", LevelMajor:
		return true
	case LevelMinor:
		return actNorm == LevelPatch || actNorm == LevelMinor
	case LevelPatch:
		return actNorm == LevelPatch
	default:
		return false
	}
}

// DiffLevel classifies the upgrade from current to available within an ecosystem
// as a patch, minor, or major bump.
//
// Rules:
//   - Major bump if epoch increased, major segment increased, or baseline is unknown.
//   - Minor bump if minor segment increased (with equal major and epoch).
//   - Patch bump otherwise (patch/4th-segment/revision/pre-release/post-release bumps or non-upgrades).
func DiffLevel(ecosystem, current, available string) BumpLevel {
	cur := strings.TrimSpace(current)
	avail := strings.TrimSpace(available)

	if strings.EqualFold(cur, avail) {
		return LevelPatch
	}
	if avail == "" || strings.EqualFold(avail, "unknown") || strings.EqualFold(avail, "dev") {
		return LevelPatch
	}
	if cur == "" || strings.EqualFold(cur, "unknown") || strings.EqualFold(cur, "dev") {
		return LevelMajor
	}

	curClean := buildMetadataRegex.ReplaceAllString(cur, "")
	availClean := buildMetadataRegex.ReplaceAllString(avail, "")

	eco := strings.ToLower(strings.TrimSpace(ecosystem))
	var curCore, availCore string

	switch eco {
	case "uv", "python", "pypi", "pipx":
		e1, curRest := extractEpoch(curClean, pep440EpochRegex)
		e2, availRest := extractEpoch(availClean, pep440EpochRegex)
		if e2 > e1 {
			return LevelMajor
		}
		if e2 < e1 {
			return LevelPatch
		}
		curCore, availCore = curRest, availRest

	case "apt", "deb", "debian", "dnf", "zypper", "rpm", "pacman", "arch":
		e1, curRest := extractEpoch(curClean, brewEpochRegex)
		e2, availRest := extractEpoch(availClean, brewEpochRegex)
		if e2 > e1 {
			return LevelMajor
		}
		if e2 < e1 {
			return LevelPatch
		}
		curCore, _ = splitPkgRelease(curRest)
		availCore, _ = splitPkgRelease(availRest)

	case "brew", "homebrew":
		e1, curRest := extractEpoch(curClean, brewEpochRegex)
		e2, availRest := extractEpoch(availClean, brewEpochRegex)
		if e2 > e1 {
			return LevelMajor
		}
		if e2 < e1 {
			return LevelPatch
		}
		_, curCore = extractRevision(curRest)
		_, availCore = extractRevision(availRest)

	default:
		curCore, availCore = curClean, availClean
	}

	p1 := parseVersion(curCore)
	p2 := parseVersion(availCore)

	if len(p1.segments) == 0 || len(p2.segments) == 0 {
		if Compare(ecosystem, current, available) < 0 {
			return LevelMajor
		}
		return LevelPatch
	}

	seg := func(p parsedVersion, idx int) int {
		if idx < len(p.segments) {
			return p.segments[idx]
		}
		return 0
	}

	maj1, maj2 := seg(p1, 0), seg(p2, 0)
	if maj2 > maj1 {
		return LevelMajor
	}
	if maj2 < maj1 {
		return LevelPatch
	}

	min1, min2 := seg(p1, 1), seg(p2, 1)
	if min2 > min1 {
		return LevelMinor
	}
	if min2 < min1 {
		return LevelPatch
	}

	return LevelPatch
}
