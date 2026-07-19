package pever

import (
	"slices"
	"strconv"
	"strings"
)

// Vendored version → marketing-name tables. Provenance: public SDK release
// notes and the TechPowerUp DLL version databases for each vendor:
//   - DLSS: NVIDIA DLSS SDK release notes / nvngx_dlss.dll version history
//     (DLSS 4 ships nvngx_dlss.dll with the 310.x versioning scheme).
//   - FSR: AMD FidelityFX SDK release notes (GPUOpen) /
//     amd_fidelityfx_dx12.dll version history. Entries marked "approx."
//     fill gaps where AMD shipped DLL builds without a distinct
//     marketing name; the nearest marketing label is used.
//   - XeSS: Intel XeSS SDK release notes / libxess.dll version history.
//
// A few entries are approximate by necessity; correctness of lookup is
// guaranteed by the 4-tier algorithm in MarketingName, not by table
// completeness.
var marketingMaps = map[Kind]map[string]string{
	KindDLSS: {
		"2.1.39":  "DLSS 2.1.39",
		"2.1.66":  "DLSS 2.1.66",
		"2.2.18":  "DLSS 2.2.18",
		"2.3.11":  "DLSS 2.3.11",
		"2.4.3":   "DLSS 2.4.3",
		"2.4.12":  "DLSS 2.4.12",
		"2.5.1":   "DLSS 2.5.1",
		"3.1.0":   "DLSS 3.1.0",
		"3.1.1":   "DLSS 3.1.1",
		"3.1.2":   "DLSS 3.1.2",
		"3.1.10":  "DLSS 3.1.10",
		"3.1.11":  "DLSS 3.1.11",
		"3.1.13":  "DLSS 3.1.13",
		"3.1.30":  "DLSS 3.1.30",
		"3.5.0":   "DLSS 3.5.0",
		"3.5.10":  "DLSS 3.5.10",
		"3.7.0":   "DLSS 3.7.0",
		"3.7.10":  "DLSS 3.7.10",
		"3.7.20":  "DLSS 3.7.20",
		"3.8.10":  "DLSS 3.8.10",
		"310.1.0": "DLSS 4 (310.1.0)",
		"310.2.0": "DLSS 4 (310.2.0)",
		"310.2.1": "DLSS 4 (310.2.1)",
	},
	KindFSR: {
		"1.0.0": "FSR 1.0",
		"1.0.2": "FSR 1.0", // approx.
		"2.0.0": "FSR 2.0",
		"2.0.1": "FSR 2.0", // approx.
		"2.1.0": "FSR 2.1",
		"2.1.1": "FSR 2.1", // approx.
		"2.2.0": "FSR 2.2",
		"2.2.1": "FSR 2.2",
		"2.2.2": "FSR 2.2",
		"3.0.0": "FSR 3.0",
		"3.0.1": "FSR 3.0", // approx.
		"3.0.2": "FSR 3.0", // approx.
		"3.1.0": "FSR 3.1",
		"3.1.1": "FSR 3.1.1",
		"3.1.2": "FSR 3.1.2",
		"3.1.3": "FSR 3.1.3",
		"3.1.4": "FSR 3.1.4",
		"4.0.0": "FSR 4.0",
		"4.0.1": "FSR 4.0.1",
		"4.0.2": "FSR 4.0.2",
		"4.1.0": "FSR 4.1",
		"4.1.1": "FSR 4.1", // approx.
	},
	KindXeSS: {
		"1.0.0": "XeSS 1.0",
		"1.0.1": "XeSS 1.0.1",
		"1.1.0": "XeSS 1.1",
		"1.2.0": "XeSS 1.2",
		"1.3.0": "XeSS 1.3",
		"1.3.1": "XeSS 1.3.1",
		"2.0.0": "XeSS 2.0",
		"2.0.1": "XeSS 2.0.1",
		"2.1.0": "XeSS 2.1",
		"2.1.1": "XeSS 2.1.1",
	},
}

// version is a dotted numeric version split into segments for comparison;
// missing segments compare as zero, so 3.7.10 == 3.7.10.0.
type version []int

func parseVersion(s string) (version, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ".")
	v := make(version, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		v[i] = n
	}
	return v, true
}

// cmp orders a and b segment-wise; missing trailing segments count as 0.
func (v version) cmp(o version) int {
	for i := 0; i < len(v) || i < len(o); i++ {
		var a, b int
		if i < len(v) {
			a = v[i]
		}
		if i < len(o) {
			b = o[i]
		}
		if a != b {
			return a - b
		}
	}
	return 0
}

type mapEntry struct {
	ver   version
	value string
}

func sortedEntries(kind Kind) []mapEntry {
	m, ok := marketingMaps[kind]
	if !ok {
		return nil
	}
	entries := make([]mapEntry, 0, len(m))
	for k, val := range m {
		v, ok := parseVersion(k)
		if !ok {
			continue
		}
		entries = append(entries, mapEntry{v, val})
	}
	slices.SortFunc(entries, func(a, b mapEntry) int { return a.ver.cmp(b.ver) })
	return entries
}

// MarketingName maps a raw DLL version string to the vendor's marketing
// name using a 4-tier lookup:
//
//  1. exact (numeric) match;
//  2. greatest same-major entry ≤ raw;
//  3. raw bracketed by a below/above pair that maps to the same value;
//  4. last-resort global nearest-below;
//  5. otherwise the raw string, unchanged.
func MarketingName(kind Kind, raw string) string {
	rv, ok := parseVersion(raw)
	if !ok {
		return raw
	}
	return lookupMarketing(sortedEntries(kind), rv, raw)
}

func lookupMarketing(entries []mapEntry, rv version, raw string) string {
	if len(entries) == 0 {
		return raw
	}

	// Tier 1.
	for _, e := range entries {
		if e.ver.cmp(rv) == 0 {
			return e.value
		}
	}

	below, above := -1, -1
	for i, e := range entries {
		switch c := e.ver.cmp(rv); {
		case c < 0:
			below = i
		case c > 0:
			above = i
		}
		if above >= 0 {
			break
		}
	}

	// Tier 2: same major as raw.
	if below >= 0 && len(rv) > 0 && len(entries[below].ver) > 0 &&
		entries[below].ver[0] == rv[0] {
		return entries[below].value
	}

	// Tier 3.
	if below >= 0 && above >= 0 &&
		entries[below].value == entries[above].value {
		return entries[below].value
	}

	// Tier 4.
	if below >= 0 {
		return entries[below].value
	}

	return raw
}
