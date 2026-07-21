package version

import "testing"

// TestCompare pins the ordering semantics upgrade eligibility relies on:
// leading "v" is normalized, numeric triples compare numerically, a
// pre-release is older than its release, pre-release identifiers compare
// numeric-aware, and "" is the lowest possible version (documented: an
// unknown version never looks newer than a known one).
func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal bare", "0.9.4", "0.9.4", 0},
		{"leading v normalized", "v0.9.4", "0.9.4", 0},
		{"leading v both sides", "v0.9.4", "v0.9.4", 0},
		{"numeric triple ascending", "0.9.4", "0.10.0", -1},
		{"numeric triple descending", "0.10.0", "0.9.9", 1},
		{"numeric not lexical", "0.9.9", "0.10.0", -1},
		{"pre-release older than release", "0.9.4-pre2", "0.9.4", -1},
		{"release newer than pre-release", "0.9.4", "0.9.4-pre2", 1},
		{"pre-release numeric-aware identifiers", "0.9.4-pre2", "0.9.4-pre10", -1},
		{"pre-release numeric-aware descending", "0.9.4-pre10", "0.9.4-pre2", 1},
		{"pre-release equal", "0.9.4-pre2", "0.9.4-pre2", 0},
		{"empty is lowest", "", "0.9.4", -1},
		{"empty is lowest vs v-tag", "", "v0.9.4", -1},
		{"non-empty beats empty", "0.1.0", "", 1},
		{"both empty equal", "", "", 0},
		{"shorter core equals padded", "0.9", "0.9.0", 0},
		{"shorter core lower", "0.9", "0.9.1", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compare(tt.a, tt.b); got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestCompareAntisymmetry: swapping the operands must flip the sign for
// every pair — a comparator that breaks antisymmetry poisons sorts and
// eligibility checks alike.
func TestCompareAntisymmetry(t *testing.T) {
	pairs := [][2]string{
		{"0.9.4", "0.10.0"},
		{"v0.9.4", "0.9.4"},
		{"0.9.4-pre2", "0.9.4"},
		{"0.9.4-pre2", "0.9.4-pre10"},
		{"", "0.9.4"},
	}
	for _, p := range pairs {
		if got, rev := Compare(p[0], p[1]), Compare(p[1], p[0]); got != -rev {
			t.Errorf("Compare(%q,%q)=%d but Compare(%q,%q)=%d, want sign flip",
				p[0], p[1], got, p[1], p[0], rev)
		}
	}
	t.Log("comparator antisymmetric on all probed pairs")
}
