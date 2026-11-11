package releasecheck

import (
	"strconv"
	"strings"
)

// semver is the parsed form of the T-0173 version grammar: a numeric
// X.Y.Z triple plus an optional prerelease.
type semver struct {
	maj, min, pat int
	pre           string
	hasPre        bool
}

// parseVersion parses the single T-0173 version grammar: exactly one
// optional leading "v", then X.Y.Z (all three segments required), then an
// optional "-prerelease". Build metadata after "+" is cut first. ok is
// false for anything else ("1.2", "v1", "release-1.2.3", empty strings).
func parseVersion(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var sv semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		sv.pre = s[i+1:]
		sv.hasPre = true
		s = s[:i]
		if sv.pre == "" {
			return semver{}, false
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for i, p := range parts {
		if !isDigits(p) {
			return semver{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, false
		}
		switch i {
		case 0:
			sv.maj = n
		case 1:
			sv.min = n
		case 2:
			sv.pat = n
		}
	}
	return sv, true
}

// comparable applies the T-0173 comparability gate to the current
// version: exactly one optional leading "v" followed by plain X.Y.Z, or
// the same base with a git-describe suffix ("1.2.4-37-g29111c6"), whose
// comparable base is the tagged X.Y.Z. Prerelease ("1.2.3-rc1"), build
// metadata ("1.2.3+build"), malformed describes and anything else are
// NOT comparable.
func comparable(version string) (semver, bool) {
	if strings.Contains(version, "+") {
		return semver{}, false
	}
	if base, ok := describeBase(version); ok {
		version = base
	}
	sv, ok := parseVersion(version)
	if !ok || sv.hasPre {
		return semver{}, false
	}
	return sv, true
}

// describeBase recognizes git-describe output "[v]X.Y.Z-<distance>-g<hash>"
// and returns the tagged base "[v]X.Y.Z". The distance must be all digits
// and the commit abbreviation a "g" prefix followed by hex digits;
// anything else (prereleases, malformed describes) is not a describe
// output and leaves the version untouched for the normal gate.
func describeBase(version string) (string, bool) {
	head, tail, found := strings.Cut(version, "-")
	if !found {
		return "", false
	}
	if _, ok := parseVersion(head); !ok {
		return "", false
	}
	dist, hash, found := strings.Cut(tail, "-")
	if !found || dist == "" || !isDigits(dist) {
		return "", false
	}
	if len(hash) < 2 || hash[0] != 'g' || !isHex(hash[1:]) {
		return "", false
	}
	return head, true
}

// compareSemver orders by numeric triple, then puts a prerelease BEFORE
// the plain release, then compares prerelease ids per dot-segment:
// numeric when both are all-digits (semver §11.4.3, "2" < "10"),
// otherwise ASCII; a shorter prefix sorts first.
func compareSemver(a, b semver) int {
	if c := cmpInt(a.maj, b.maj); c != 0 {
		return c
	}
	if c := cmpInt(a.min, b.min); c != 0 {
		return c
	}
	if c := cmpInt(a.pat, b.pat); c != 0 {
		return c
	}
	switch {
	case a.hasPre && !b.hasPre:
		return -1
	case !a.hasPre && b.hasPre:
		return 1
	case !a.hasPre:
		return 0
	}
	return comparePrerelease(a.pre, b.pre)
}

// comparePrerelease compares two dot-separated prerelease strings.
func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := comparePreID(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(as), len(bs))
}

// comparePreID compares one prerelease id: numeric when both sides are
// all-digits, ASCII otherwise.
func comparePreID(a, b string) int {
	if isDigits(a) && isDigits(b) {
		na, _ := strconv.Atoi(a)
		nb, _ := strconv.Atoi(b)
		return cmpInt(na, nb)
	}
	return strings.Compare(a, b)
}

// isHex reports whether s is a non-empty run of ASCII hex digits.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// cmpInt compares two ints: -1, 0 or 1.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
