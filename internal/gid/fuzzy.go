package gid

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// editionPhrases are multi-word marketing suffixes removed before token
// matching ("Fallout 4 Game of the Year Edition" → "fallout 4").
var editionPhrases = []string{
	"game of the year", "directors cut", "definitive edition",
	"enhanced edition", "complete edition", "anniversary edition",
	"collectors edition", "legacy edition", "gold edition",
	"history edition", "ultimate edition", "special edition",
}

// editionTokens are single-word edition/repack markers.
var editionTokens = map[string]bool{
	"remake": true, "remastered": true, "remaster": true, "goty": true,
	"definitive": true, "enhanced": true, "complete": true, "edition": true,
	"hd": true, "anniversary": true, "collection": true, "proper": true,
	"repack": true, "fitgirl": true, "dodi": true, "dlc": true,
	"update": true, "history": true, "gold": true, "legendary": true,
	"ultimate": true, "redux": true, "special": true,
}

// platformTokens are store/platform noise words.
var platformTokens = map[string]bool{
	"win": true, "windows": true, "win64": true, "win32": true,
	"x64": true, "x86": true, "x86_64": true, "amd64": true,
	"dx9": true, "dx10": true, "dx11": true, "dx12": true,
	"vk": true, "vulkan": true, "steam": true, "gog": true,
	"epic": true, "egs": true, "msstore": true, "wingdk": true,
	"linux": true, "mac": true, "proton": true,
}

var (
	bracketRe    = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\{[^}]*\}`)
	versionRunRe = regexp.MustCompile(`\bv\d+(?:[. ]\d+)+\b|\b\d+\.\d+(?:\.\d+)*`)
	versionTokRe = regexp.MustCompile(`^(v\d+|multi\d+)$`)
)

// Normalize turns a raw title candidate into a matching key:
// compatibility-normalized, case-folded, decorations/brackets/edition and
// platform noise removed, separators collapsed. It is for MATCHING ONLY —
// never for display.
func Normalize(s string) string {
	return normalize(s, true)
}

// normalizeKeepEdition is the normalize pipeline minus edition stripping,
// used to detect edition mismatches between candidate and store item.
func normalizeKeepEdition(s string) string { return normalize(s, false) }

func normalize(s string, stripEdition bool) string {
	// Canonical decomposition drops diacritics (Ragnarök → ragnarok)
	// while keeping ™/® as themselves for the literal strip below —
	// NFKC would explode ™ into the letters "tm".
	s = norm.NFD.String(s)
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, s)
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	s = strings.NewReplacer("™", "", "®", "", "©", "", "'", "", "’", "").Replace(s)
	s = versionRunRe.ReplaceAllString(s, " ")
	s = bracketRe.ReplaceAllString(s, " ")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '_', '.', '-', ':', ';', ',', '!', '?':
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if stripEdition {
		for _, p := range editionPhrases {
			s = strings.ReplaceAll(s, " "+p+" ", " ")
			s = strings.TrimPrefix(s, p+" ")
			s = strings.TrimSuffix(s, " "+p)
		}
	}
	toks := strings.Fields(s)
	out := toks[:0]
	for _, t := range toks {
		if platformTokens[t] || versionTokRe.MatchString(t) {
			continue
		}
		if stripEdition && editionTokens[t] {
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, " ")
}

// Score ranks one store item against the raw title candidate: exact
// normalized equality dominates (+100), strict token-set near-equality
// adds (+70), a PC platform adds (+10), an edition mismatch between the
// two raw titles subtracts (−20). Candidates of 4 characters or fewer
// only ever earn the exact-match credit — fuzzy acceptance at that length
// is how "Doom" becomes "Doom 3".
func Score(candRaw, itemName string, pc bool) int {
	candNorm := Normalize(candRaw)
	itemNorm := Normalize(itemName)
	score := 0
	if candNorm != "" && candNorm == itemNorm {
		score += 100
	} else if len(candNorm) > 4 && tokenSetRatio(candNorm, itemNorm) >= 90 {
		score += 70
	}
	if pc {
		score += 10
	}
	if editionMismatch(candRaw, itemName) {
		score -= 20
	}
	return score
}

// tokenSetRatio is the strict Jaccard similarity (×100) of the two token
// sets. Deliberately not a coverage/subset ratio: "Frostpunk" ⊂
// "Frostpunk 2" must not score as a near-match.
func tokenSetRatio(a, b string) int {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, t := range strings.Fields(s) {
			m[t] = true
		}
		return m
	}
	sa, sb := set(a), set(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter, union := 0, len(sa)
	for t := range sb {
		if sa[t] {
			inter++
		} else {
			union++
		}
	}
	return inter * 100 / union
}

// editionMismatch reports whether exactly one of the two raw titles
// carries edition tokens (the candidate is already normalized; the item
// is normalized with editions kept for this check).
func editionMismatch(candNorm, itemName string) bool {
	has := func(s string) bool {
		for _, t := range strings.Fields(normalizeKeepEdition(s)) {
			if editionTokens[t] {
				return true
			}
		}
		return false
	}
	return has(candNorm) != has(itemName)
}

// Accept encodes the identification threshold: a score of 90 or better
// stands alone; 75-89 needs a second corroborating signal (e.g. the store
// item's developer matching the PE CompanyName).
func Accept(score int, corroborated bool) bool {
	if score >= 90 {
		return true
	}
	return score >= 75 && corroborated
}
