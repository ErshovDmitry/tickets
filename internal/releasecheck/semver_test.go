package releasecheck

import "testing"

// TestParseVersionGrammar pins the single T-0173 grammar: one optional
// leading "v", then X.Y.Z (all three segments), optional "-prerelease";
// "+" build metadata is cut before parsing.
func TestParseVersionGrammar(t *testing.T) {
	ok := []string{"1.2.3", "v1.2.3", "0.0.0", "1.2.3-rc1", "1.2.3-2", "v1.2.3-alpha.1", "1.2.3+build", "v1.2.3+build"}
	for _, s := range ok {
		if _, ok := parseVersion(s); !ok {
			t.Errorf("parseVersion(%q): want ok, got !ok", s)
		}
	}
	bad := []string{"", "dev", "1.2", "v1", "v", "1.2.3.4", "release-1.2.3", "1.x.3", "1.2.3-", "vv1.2.3"}
	for _, s := range bad {
		if _, ok := parseVersion(s); ok {
			t.Errorf("parseVersion(%q): want !ok, got ok", s)
		}
	}
}

// TestCompareVersions covers the T-0173 semver table with concrete
// values, exercising the live comparator compareSemver on parseVersion
// results — exactly what production compareResult does.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"v-prefix ignored", "v1.2.4", "1.2.4", 0},
		{"build metadata ignored", "1.2.3+build", "1.2.3", 0},
		{"numeric patch", "v1.2.4", "1.2.10", -1},
		{"numeric minor", "1.2.0", "1.10.0", -1},
		{"numeric major", "1.0.0", "2.0.0", -1},
		{"greater", "1.2.10", "1.2.4", 1},
		{"prerelease before release", "1.3.0-rc1", "1.3.0", -1},
		{"numeric prerelease ids", "1.2.3-2", "1.2.3-10", -1},
		{"ascii prerelease ids", "1.0.0-alpha", "1.0.0-beta", -1},
		{"shorter prerelease prefix", "1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"equal", "2.0.0", "2.0.0", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := parseVersion(tc.a)
			if !ok {
				t.Fatalf("parseVersion(%q): !ok", tc.a)
			}
			y, ok := parseVersion(tc.b)
			if !ok {
				t.Fatalf("parseVersion(%q): !ok", tc.b)
			}
			got := compareSemver(x, y)
			norm := got
			if norm > 0 {
				norm = 1
			} else if norm < 0 {
				norm = -1
			}
			if norm != tc.want {
				t.Errorf("compareSemver(%q, %q) = %d, want sign %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestCompareResultUnparsable proves the live no-silent-success hook
// (T-0173 flow d): an unparsable latest tag makes the production
// compareResult report !ok, so Check fails instead of caching it OK.
func TestCompareResultUnparsable(t *testing.T) {
	cur, ok := comparable("1.2.3")
	if !ok {
		t.Fatal("comparable(1.2.3): !ok")
	}
	for _, tag := range []string{"1.2", "garbage", ""} {
		res, ok := compareResult(tag, "u", cur)
		if ok || res != (Result{}) {
			t.Errorf("compareResult(%q) = %+v, ok=%v; want zero Result, !ok", tag, res, ok)
		}
	}
}

// TestComparableGate pins the T-0173 comparability gate for the current
// version: plain X.Y.Z and git-describe output ("[v]X.Y.Z-<distance>-
// g<hash>") are comparable; unknown dev, bare hash, malformed describes,
// prerelease and build metadata are NOT.
func TestComparableGate(t *testing.T) {
	ok := []string{
		"1.2.3", "v1.2.3", "0.0.0",
		"1.2.4-36-gbeb07c8", "v1.2.4-37-g29111c6", "1.2.4-0-gabc123",
	}
	for _, s := range ok {
		if _, ok := comparable(s); !ok {
			t.Errorf("comparable(%q): want ok", s)
		}
	}
	bad := []string{
		"", "dev", "garbage", "29111c6",
		"1.2", "v1",
		"1.2.3-rc1", "1.2.3+build", "v1.2.3+build",
		"1.2.4-37", "1.2.4-37-", "1.2.4-37-g", "1.2.4-37-gZZ",
		"1.2.4-abc-g123", "1.2.4-rc1-gabc", "1.2.4-37-g29111c6-extra",
	}
	for _, s := range bad {
		if _, ok := comparable(s); ok {
			t.Errorf("comparable(%q): want !ok", s)
		}
	}
}

// TestComparableGitDescribeBase pins the T-0173 root-verification fix: a
// git-describe current version compares as its tagged base X.Y.Z, so a
// standard build like 1.2.4-37-g29111c6 is no longer reported as Dev.
func TestComparableGitDescribeBase(t *testing.T) {
	cases := []struct {
		version string
		base    string
	}{
		{"1.2.4-37-g29111c6", "1.2.4"},
		{"v1.2.4-37-g29111c6", "v1.2.4"},
		{"1.2.4-0-gabc123", "1.2.4"},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			got, ok := comparable(tc.version)
			if !ok {
				t.Fatalf("comparable(%q): want ok", tc.version)
			}
			want, ok := parseVersion(tc.base)
			if !ok {
				t.Fatalf("parseVersion(%q): !ok", tc.base)
			}
			if got != want {
				t.Errorf("comparable(%q) = %+v, want base %+v", tc.version, got, want)
			}
		})
	}
}
