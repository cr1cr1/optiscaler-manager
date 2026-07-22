// Package version compares OptiScaler version strings — release tags
// ("v0.9.4", "0.10.0-pre1") and on-disk evidence versions ("0.9.4") — for
// upgrade eligibility. It is deliberately small and dependency-free; it is
// NOT a full semver implementation, just the ordering the manager needs.
package version

import "strings"

// Compare orders two version strings, returning -1, 0, or +1.
//
// Semantics:
//   - one leading "v"/"V" is ignored: "v0.9.4" == "0.9.4";
//   - the numeric core compares segment by segment, numerically where both
//     segments are digits ("0.9.4" < "0.10.0"); missing trailing segments
//     count as zero ("0.9" == "0.9.0");
//   - a pre-release suffix (everything after the first "-") is OLDER than
//     the plain release: "0.9.4-pre2" < "0.9.4"; two suffixes compare
//     numeric-aware (digit runs numerically, byte-wise otherwise), so
//     "0.9.4-pre2" < "0.9.4-pre10";
//   - "" is the lowest version of all (documented): an unknown version
//     never satisfies "installed is older than target", which keeps
//     upgrade offers off rows whose version evidence ran dry.
func Compare(a, b string) int {
	a, b = normalize(a), normalize(b)
	if a == "" || b == "" {
		switch a {
		case b:
			return 0
		case "":
			return -1
		default:
			return 1
		}
	}
	aCore, aPre := splitPre(a)
	bCore, bPre := splitPre(b)
	if c := compareCore(aCore, bCore); c != 0 {
		return c
	}
	if aPre == "" || bPre == "" {
		switch aPre {
		case bPre:
			return 0
		case "":
			return 1 // the plain release beats any of its pre-releases
		default:
			return -1
		}
	}
	return naturalCompare(aPre, bPre)
}

// normalize trims space and strips one leading v/V.
func normalize(v string) string {
	v = strings.TrimSpace(v)
	if v != "" && (v[0] == 'v' || v[0] == 'V') {
		v = v[1:]
	}
	return v
}

// splitPre cuts v at the first "-" into numeric core and pre-release.
func splitPre(v string) (core, pre string) {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// compareCore compares dot-separated segments pairwise; a missing segment
// is zero, so "0.9" and "0.9.0" are equal.
func compareCore(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := compareSegment(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// compareSegment orders two core segments: both numeric → numeric order,
// otherwise plain byte order.
func compareSegment(a, b string) int {
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	if isDigits(a) && isDigits(b) {
		return compareNumeric(a, b)
	}
	return strings.Compare(a, b)
}

// compareNumeric orders two all-digit strings without integer overflow:
// leading zeros are insignificant, then longer wins, then byte order.
func compareNumeric(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// naturalCompare walks both strings run by run: digit runs compare
// numerically, anything else byte-wise; a digit run sorts before a
// non-digit run (semver: numeric identifiers beat alphanumeric ones in
// precedence), and a strict prefix sorts before its extension.
func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		da, db := isDigit(a[i]), isDigit(b[j])
		switch {
		case da && db:
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if c := compareNumeric(a[si:i], b[sj:j]); c != 0 {
				return c
			}
		case da != db:
			if da {
				return -1
			}
			return 1
		default:
			if a[i] != b[j] {
				if a[i] < b[j] {
					return -1
				}
				return 1
			}
			i++
			j++
		}
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	default:
		return 0
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
