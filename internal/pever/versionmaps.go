package pever

import (
	"slices"
	"strconv"
	"strings"
)

// Vendored version → marketing-name tables.
//
// Provenance: Agustinm28/Optiscaler-Client (the reference OptiScaler
// manager), assets/configs/{fsr,dlss,xess}_version_map.json, fetched
// 2026-07-20 from the main branch (identical on development):
//   - https://raw.githubusercontent.com/Agustinm28/Optiscaler-Client/main/assets/configs/fsr_version_map.json   (22 entries)
//   - https://raw.githubusercontent.com/Agustinm28/Optiscaler-Client/main/assets/configs/dlss_version_map.json  (23 entries)
//   - https://raw.githubusercontent.com/Agustinm28/Optiscaler-Client/main/assets/configs/xess_version_map.json  (10 entries)
//
// Keys are vendored VERBATIM from those files: they are the DLLs' actual
// raw PE file versions (FSR 3.1.4 ships amd_fidelityfx_*.dll as
// 1.0.1.41314, FSR 4.1 as 2.2.0.1328, ...), NOT marketing numbers.
// Values are the reference's bare release numbers with the vendor prefix
// prepended per this package's output convention ("3.1.4" → "FSR 3.1.4").
var marketingMaps = map[Kind]map[string]string{
	KindDLSS: {
		"310.6.0":  "DLSS 4.5",
		"310.5.3":  "DLSS 4.5",
		"310.5.0":  "DLSS 4.5",
		"310.4.0":  "DLSS 4.0",
		"310.2.1":  "DLSS 4.0",
		"310.0.0":  "DLSS 4.0",
		"3.8.10.0": "DLSS 3.8.10",
		"3.7.20.0": "DLSS 3.7.20",
		"3.7.0.0":  "DLSS 3.7",
		"3.5.10.0": "DLSS 3.5.10",
		"3.5.0.0":  "DLSS 3.5",
		"3.1.30.0": "DLSS 3.1.30",
		"3.1.13.0": "DLSS 3.1.13",
		"3.1.1.0":  "DLSS 3.1.1",
		"2.5.1.0":  "DLSS 2.5.1",
		"2.4.3.0":  "DLSS 2.4.3",
		"2.4.2.0":  "DLSS 2.4.2",
		"2.4.1.0":  "DLSS 2.4.1",
		"2.4.0.0":  "DLSS 2.4",
		"2.3.10.0": "DLSS 2.3.10",
		"2.3.9.0":  "DLSS 2.3.9",
		"2.3.8.0":  "DLSS 2.3.8",
		"2.2.6.0":  "DLSS 2.2.6",
	},
	KindFSR: {
		"2.2.0.1328":  "FSR 4.1",
		"2.1.0.968":   "FSR 4.1",
		"1.8.1.1042":  "FSR 4.0.3",
		"1.2.1.8845":  "FSR 4.0.2",
		"1.0.1.50230": "FSR 4.0",
		"1.0.1.41314": "FSR 3.1.4",
		"1.0.1.40157": "FSR 3.1.3",
		"1.0.1.39157": "FSR 3.1.3",
		"1.0.2.38022": "FSR 3.1.2",
		"1.0.1.38338": "FSR 3.1.2",
		"1.0.1.37507": "FSR 3.1.1",
		"1.0.0.36752": "FSR 3.1",
		"1.0.0.36604": "FSR 3.1",
		"1.0.0.36208": "FSR 3.1",
		"1.0.0.3340":  "FSR 2.2.1",
		"1.0.0.3204":  "FSR 2.2.1",
		"1.0.0.3160":  "FSR 2.2",
		"1.0.0.2458":  "FSR 2.1.2",
		"1.0.0.2240":  "FSR 2.1.1",
		"1.0.0.2163":  "FSR 2.1",
		"1.0.0.1423":  "FSR 2.0.1",
		"1.0.0.1301":  "FSR 2.0",
	},
	KindXeSS: {
		"2.1.0.450": "XeSS 2.1",
		"2.0.0.120": "XeSS 2.0",
		"1.5.2.85":  "XeSS 1.5.2",
		"1.5.0.32":  "XeSS 1.5",
		"1.3.1.2":   "XeSS 1.3.1",
		"1.3.0.11":  "XeSS 1.3",
		"1.2.0.13":  "XeSS 1.2",
		"1.1.0.10":  "XeSS 1.1",
		"1.0.1.11":  "XeSS 1.0.1",
		"1.0.0.7":   "XeSS 1.0",
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
