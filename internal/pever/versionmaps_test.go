package pever

import "testing"

func TestMarketingNameLookup_FourTier(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		raw  string
		want string
	}{
		// Tier 1: exact match.
		{"tier1 exact dlss", KindDLSS, "3.7.10", "DLSS 3.7.10"},
		{"tier1 exact with trailing zero", KindDLSS, "3.7.10.0", "DLSS 3.7.10"},
		{"tier1 exact fsr", KindFSR, "3.1.0", "FSR 3.1"},
		{"tier1 exact xess", KindXeSS, "1.3.0", "XeSS 1.3"},
		{"tier1 v-prefix tolerated", KindDLSS, "v3.7.10", "DLSS 3.7.10"},

		// Tier 2: same-major entry ≤ raw wins over a different-major
		// nearest-below.
		{"tier2 same major below", KindDLSS, "3.7.15", "DLSS 3.7.10"},
		{"tier2 fsr same major", KindFSR, "3.1.6", "FSR 3.1.4"},

		// Tier 3: raw between two keys that map to the same marketing
		// value → that value even without a same-major entry.
		{"tier3 bracketed same value", KindFSR, "2.2.3", "FSR 2.2"},

		// Tier 4: last-resort global nearest-below (different major,
		// bracketing values differ).
		{"tier4 nearest below", KindDLSS, "3.0.5", "DLSS 2.5.1"},

		// Tier 5: below every known key (or unknown) → raw unchanged.
		{"tier5 too old returns raw", KindDLSS, "1.5.0", "1.5.0"},
		{"tier5 garbage returns raw", KindDLSS, "not-a-version", "not-a-version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MarketingName(tc.kind, tc.raw); got != tc.want {
				t.Errorf("MarketingName(%v, %q) = %q, want %q", tc.kind, tc.raw, got, tc.want)
			}
		})
	}

	t.Run("tier3 bracketed same value cross-major", func(t *testing.T) {
		entries := []mapEntry{
			{version{1, 0}, "SameName"},
			{version{3, 0}, "SameName"},
			{version{4, 0}, "Other"},
		}
		rv, ok := parseVersion("2.5")
		if !ok {
			t.Fatal("parseVersion")
		}
		if got := lookupMarketing(entries, rv, "2.5"); got != "SameName" {
			t.Errorf("got %q, want %q", got, "SameName")
		}
	})

	t.Run("tier4 bracketed differing values", func(t *testing.T) {
		entries := []mapEntry{
			{version{1, 0}, "Old"},
			{version{3, 0}, "New"},
		}
		rv, ok := parseVersion("2.5")
		if !ok {
			t.Fatal("parseVersion")
		}
		if got := lookupMarketing(entries, rv, "2.5"); got != "Old" {
			t.Errorf("got %q, want %q", got, "Old")
		}
	})
}
