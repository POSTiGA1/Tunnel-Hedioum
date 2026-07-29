package sysutil

import (
	"strconv"
	"strings"
)

// IsNewer reports whether candidate is a strictly newer semantic version than
// current. Both may carry a leading 'v' and an optional pre-release/build suffix
// (which is ignored). If either cannot be parsed, it falls back to inequality
// (update when the tags simply differ) so an odd/dev current version still
// updates to a real release.
func IsNewer(candidate, current string) bool {
	cp, ok1 := parseSemver(candidate)
	cc, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return candidate != "" && candidate != current
	}
	for i := 0; i < 3; i++ {
		if cp[i] != cc[i] {
			return cp[i] > cc[i]
		}
	}
	return false
}

// parseSemver parses "vMAJOR[.MINOR[.PATCH]]" (missing parts = 0), ignoring any
// -prerelease or +build suffix.
func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return [3]int{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
