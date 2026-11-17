package upgrade

import (
	"path"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/version"
)

// MatchExclude reports whether pkg matches any glob pattern in patterns.
// Patterns may be comma-separated and are matched case-insensitively against
// pkg.Id, pkg.Name, and pkg.ManagerName + ":" + pkg.Id.
// Returns the first matching pattern and true, or ("", false) if none match.
func MatchExclude(pkg pm.PackageUpdate, patterns []string) (string, bool) {
	if len(patterns) == 0 {
		return "", false
	}

	var candidates []string
	if pkg.Id != "" {
		candidates = append(candidates, pkg.Id)
	}
	if pkg.Name != "" && !strings.EqualFold(pkg.Name, pkg.Id) {
		candidates = append(candidates, pkg.Name)
	}
	if pkg.ManagerName != "" && pkg.Id != "" {
		candidates = append(candidates, pkg.ManagerName+":"+pkg.Id)
	}
	if len(candidates) == 0 {
		return "", false
	}

	for _, raw := range patterns {
		for _, part := range strings.Split(raw, ",") {
			pat := strings.TrimSpace(part)
			if pat == "" {
				continue
			}
			for _, cand := range candidates {
				if globMatchFold(pat, cand) {
					return pat, true
				}
			}
		}
	}
	return "", false
}

// globMatchFold matches pattern against target case-insensitively.
// Supports '*', '?', and '[...]' character classes. Unlike path.Match,
// '*' and '?' match '/' inside scoped package IDs (e.g. npm:@scope/pkg).
func globMatchFold(pattern, target string) bool {
	pLower := strings.ToLower(pattern)
	tLower := strings.ToLower(target)

	if strings.ContainsAny(pLower, "[]") {
		tMatch := tLower
		if !strings.Contains(pLower, "/") && strings.Contains(tMatch, "/") {
			tMatch = strings.ReplaceAll(tMatch, "/", "\x01")
		}
		matched, err := path.Match(pLower, tMatch)
		if err == nil {
			return matched
		}
		return pLower == tLower
	}

	return wildcardMatch([]rune(pLower), []rune(tLower))
}

func wildcardMatch(p, t []rune) bool {
	pi, ti := 0, 0
	starIdx, matchIdx := -1, 0

	for ti < len(t) {
		if pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]) {
			pi++
			ti++
		} else if pi < len(p) && p[pi] == '*' {
			starIdx = pi
			matchIdx = ti
			pi++
		} else if starIdx != -1 {
			pi = starIdx + 1
			matchIdx++
			ti = matchIdx
		} else {
			return false
		}
	}

	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// FilterPolicies partitions pkgs into allowed targets and policy-skipped results
// based on exclude glob patterns and the maximum allowed semver bump level.
func FilterPolicies(pkgs []pm.PackageUpdate, excludePatterns []string, maxLevel string) (allowed []pm.PackageUpdate, skipped []Result, err error) {
	parsedMax, err := version.ParseLevel(maxLevel)
	if err != nil {
		return nil, nil, err
	}

	for _, pkg := range pkgs {
		if _, excluded := MatchExclude(pkg, excludePatterns); excluded {
			skipped = append(skipped, Result{
				Package: pkg,
				Status:  StatusSkippedExcluded,
			})
			continue
		}

		if parsedMax != "" && parsedMax != version.LevelMajor {
			if pkg.Version != "" || pkg.AvailableVersion != "" {
				actual := version.DiffLevel(pkg.ManagerName, pkg.Version, pkg.AvailableVersion)
				if !version.AllowsLevel(parsedMax, actual) {
					skipped = append(skipped, Result{
						Package: pkg,
						Status:  StatusSkippedLevel,
					})
					continue
				}
			}
		}

		allowed = append(allowed, pkg)
	}

	return allowed, skipped, nil
}
