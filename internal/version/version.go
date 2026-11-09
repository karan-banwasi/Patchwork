package version

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Pre-release phase weights (lower means earlier/older)
const (
	phaseDev     = -2.0
	phaseAlpha   = -1.3
	phaseBeta    = -1.2
	phasePreview = -1.15
	phaseRC      = -1.1
	phaseFinal   = 0.0
	phasePost    = 1.0
)

var (
	// PEP 440 epoch: e.g. "1!2.0.0"
	pep440EpochRegex = regexp.MustCompile(`^(\d+)!`)
	// Debian/Brew epoch: e.g. "1:2.0.0"
	brewEpochRegex = regexp.MustCompile(`^(\d+):`)
	// Brew revision: e.g. "1.16.0_1"
	brewRevisionRegex = regexp.MustCompile(`_(\d+)$`)
	// SemVer build metadata: e.g. "+build.123"
	buildMetadataRegex = regexp.MustCompile(`\+[0-9A-Za-z.\-_]+`)
	// Pre-release tag regex: matches optional separator followed by tag and optional integer
	phaseTagRegex = regexp.MustCompile(`(?i)([._-]?(?:dev|alpha|preview|beta|candidate|post|rev)|(?:[._-]|\d)(?:rc|[abcr]))([0-9]*)`)
)

// Compare returns -1 if v1 < v2 (v2 is newer), 0 if v1 == v2, and 1 if v1 > v2 (v1 is newer).
func Compare(ecosystem, v1, v2 string) int {
	v1 = strings.TrimSpace(v1)
	v2 = strings.TrimSpace(v2)

	if strings.EqualFold(v1, v2) {
		return 0
	}
	if v1 == "" {
		return -1
	}
	if v2 == "" {
		return 1
	}

	// Handle special sentinel values
	if strings.EqualFold(v1, "unknown") || strings.EqualFold(v1, "dev") {
		return -1
	}
	if strings.EqualFold(v2, "unknown") || strings.EqualFold(v2, "dev") {
		return 1
	}

	eco := strings.ToLower(ecosystem)

	// Strip build metadata (+...) before comparing
	v1Clean := buildMetadataRegex.ReplaceAllString(v1, "")
	v2Clean := buildMetadataRegex.ReplaceAllString(v2, "")

	switch eco {
	case "uv", "python", "pypi":
		return comparePEP440(v1Clean, v2Clean)
	case "brew", "homebrew":
		return compareBrew(v1Clean, v2Clean)
	default:
		return compareGeneric(v1Clean, v2Clean)
	}
}

// IsUpgrade reports whether available is strictly newer than current (current < available).
func IsUpgrade(ecosystem, current, available string) bool {
	return Compare(ecosystem, current, available) < 0
}

func compareBrew(v1, v2 string) int {
	// Parse epochs
	e1, v1Rest := extractEpoch(v1, brewEpochRegex)
	e2, v2Rest := extractEpoch(v2, brewEpochRegex)
	if e1 != e2 {
		if e1 < e2 {
			return -1
		}
		return 1
	}

	// Parse revisions
	r1, v1Core := extractRevision(v1Rest)
	r2, v2Core := extractRevision(v2Rest)

	cmp := compareGeneric(v1Core, v2Core)
	if cmp != 0 {
		return cmp
	}
	if r1 < r2 {
		return -1
	}
	if r1 > r2 {
		return 1
	}
	return 0
}

func comparePEP440(v1, v2 string) int {
	e1, v1Rest := extractEpoch(v1, pep440EpochRegex)
	e2, v2Rest := extractEpoch(v2, pep440EpochRegex)
	if e1 != e2 {
		if e1 < e2 {
			return -1
		}
		return 1
	}
	return compareGeneric(v1Rest, v2Rest)
}

func extractEpoch(v string, re *regexp.Regexp) (int, string) {
	if m := re.FindStringSubmatch(v); len(m) == 2 {
		epoch, _ := strconv.Atoi(m[1])
		return epoch, v[len(m[0]):]
	}
	return 0, v
}

func extractRevision(v string) (int, string) {
	if m := brewRevisionRegex.FindStringSubmatch(v); len(m) == 2 {
		rev, _ := strconv.Atoi(m[1])
		return rev, v[:len(v)-len(m[0])]
	}
	return 0, v
}

type parsedVersion struct {
	segments []int
	phase    float64
	phaseNum int
	preTag   string
}

func parseVersion(v string) parsedVersion {
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	pv := parsedVersion{phase: phaseFinal}

	core := v
	loc := phaseTagRegex.FindStringSubmatchIndex(v)
	if len(loc) >= 4 {
		start := loc[0]
		tagWithSep := strings.ToLower(v[loc[2]:loc[3]])
		if len(tagWithSep) > 0 && tagWithSep[0] >= '0' && tagWithSep[0] <= '9' {
			start++
			tagWithSep = tagWithSep[1:]
		}
		tag := strings.TrimLeft(tagWithSep, "._-")
		afterTagIdx := loc[3]

		// Single letter tags (a, b, c, r) must not be followed by letters (e.g. -release, -candidate)
		if len(tag) == 1 && afterTagIdx < len(v) && ((v[afterTagIdx] >= 'a' && v[afterTagIdx] <= 'z') || (v[afterTagIdx] >= 'A' && v[afterTagIdx] <= 'Z')) {
			if idx := strings.IndexAny(core, "-+"); idx != -1 {
				core = core[:idx]
			}
		} else {
			core = v[:start]
			numStr := v[loc[4]:loc[5]]

			switch tag {
			case "post", "rev", "r":
				pv.phase = phasePost
			case "dev":
				pv.phase = phaseDev
			case "rc", "c", "candidate":
				pv.phase = phaseRC
			case "preview":
				pv.phase = phasePreview
			case "alpha", "a":
				pv.phase = phaseAlpha
			case "beta", "b":
				pv.phase = phaseBeta
			}
			pv.preTag = tag

			if numStr != "" {
				if n, err := strconv.Atoi(numStr); err == nil {
					pv.phaseNum = n
				}
			} else {
				// Extract following number if any, e.g. -rc.1
				rest := v[loc[1]:]
				pv.phaseNum = extractTagInt(rest, "")
			}
		}
	} else if idx := strings.IndexAny(core, "-+"); idx != -1 {
		core = core[:idx]
	}

	parts := strings.Split(core, ".")
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		if n, err := strconv.Atoi(clean); err == nil {
			pv.segments = append(pv.segments, n)
		} else {
			// Extract leading digits if any
			var digits string
			for _, r := range clean {
				if unicode.IsDigit(r) {
					digits += string(r)
				} else {
					break
				}
			}
			if digits != "" {
				if n, err := strconv.Atoi(digits); err == nil {
					pv.segments = append(pv.segments, n)
				}
			}
		}
	}

	return pv
}

func extractTagInt(s, tag string) int {
	idx := strings.Index(s, tag)
	if idx == -1 {
		return 0
	}
	rest := strings.TrimLeft(s[idx+len(tag):], ".-_")
	var numStr string
	for _, r := range rest {
		if unicode.IsDigit(r) {
			numStr += string(r)
		} else {
			break
		}
	}
	if numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil {
			return n
		}
	}
	return 0
}

func compareGeneric(v1, v2 string) int {
	p1 := parseVersion(v1)
	p2 := parseVersion(v2)

	// Compare core numeric segments
	maxLen := len(p1.segments)
	if len(p2.segments) > maxLen {
		maxLen = len(p2.segments)
	}

	for i := 0; i < maxLen; i++ {
		s1 := 0
		if i < len(p1.segments) {
			s1 = p1.segments[i]
		}
		s2 := 0
		if i < len(p2.segments) {
			s2 = p2.segments[i]
		}
		if s1 < s2 {
			return -1
		}
		if s1 > s2 {
			return 1
		}
	}

	// Compare pre-release/post-release phases
	if p1.phase < p2.phase {
		return -1
	}
	if p1.phase > p2.phase {
		return 1
	}

	// If phases are identical, compare phase numbers (e.g. rc1 vs rc2)
	if p1.phaseNum < p2.phaseNum {
		return -1
	}
	if p1.phaseNum > p2.phaseNum {
		return 1
	}

	return 0
}
