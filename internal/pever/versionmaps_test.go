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
		{"tier1 exact dlss", KindDLSS, "3.7.20", "DLSS 3.7.20"},
		{"tier1 exact with trailing zero", KindDLSS, "3.7.20.0", "DLSS 3.7.20"},
		{"tier1 exact fsr", KindFSR, "1.0.2.38022", "FSR 3.1.2"},
		{"tier1 exact xess", KindXeSS, "1.3.0.11", "XeSS 1.3"},
		{"tier1 v-prefix tolerated", KindDLSS, "v3.7.20", "DLSS 3.7.20"},

		// Tier 2: same-major entry ≤ raw wins over a different-major
		// nearest-below.
		{"tier2 same major below", KindDLSS, "3.7.15", "DLSS 3.7"},
		{"tier2 fsr same major", KindFSR, "1.0.1.42000", "FSR 3.1.4"},

		// Tier 4: last-resort global nearest-below (different major,
		// bracketing values differ).
		{"tier4 nearest below", KindDLSS, "3.0.5", "DLSS 2.5.1"},
		{"tier4 fsr above every key", KindFSR, "4.1.1.0", "FSR 4.1"},

		// Tier 5: below every known key (or unknown) → raw unchanged.
		{"tier5 too old returns raw", KindDLSS, "1.5.0", "1.5.0"},
		{"tier5 garbage returns raw", KindDLSS, "not-a-version", "not-a-version"},

		// Real raw PE file versions from the reference client map
		// (Agustinm28/Optiscaler-Client assets/configs/*_version_map.json).
		// FSR DLLs ship raw 4-segment file versions, not marketing numbers.
		{"real raw fsr 3.1.4", KindFSR, "1.0.1.41314", "FSR 3.1.4"},
		{"real raw fsr 4.1", KindFSR, "2.2.0.1328", "FSR 4.1"},
		{"real raw fsr 4.0", KindFSR, "1.0.1.50230", "FSR 4.0"},
		{"real raw fsr 3.1", KindFSR, "1.0.0.36752", "FSR 3.1"},
		{"real raw fsr 2.0", KindFSR, "1.0.0.1301", "FSR 2.0"},
		{"real raw fsr between keys same major", KindFSR, "1.0.0.40000", "FSR 3.1"},
		{"real raw fsr 4.1 same-major fallback", KindFSR, "2.1.0.1000", "FSR 4.1"},
		{"real raw dlss 4.5", KindDLSS, "310.5.3", "DLSS 4.5"},
		{"real raw dlss 3.7.20", KindDLSS, "3.7.20.0", "DLSS 3.7.20"},
		{"real raw xess 2.0", KindXeSS, "2.0.0.120", "XeSS 2.0"},
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
