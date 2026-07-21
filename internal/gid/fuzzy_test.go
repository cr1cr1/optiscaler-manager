package gid

import "testing"

// Normalize turns a raw title candidate into a matching key: compatibility
// form, case, decorations, bracket tags, edition noise, platform noise,
// and separator variants are all removed.
func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Dead Space Remake":                            "dead space",
		"Assassin's Creed Shadows":                     "assassins creed shadows",
		"A Plague Tale: Requiem":                       "a plague tale requiem",
		"The Witcher 3 - Wild Hunt":                    "the witcher 3 wild hunt",
		"HELLDIVERS™ 2":                                "helldivers 2",
		"God of War Ragnarök":                          "god of war ragnarok",
		"STAR WARS™: Squadrons":                        "star wars squadrons",
		"Anno 1404 - History Edition":                  "anno 1404",
		"Cyberpunk 2077":                               "cyberpunk 2077",
		"Grand Theft Auto V":                           "grand theft auto v",
		"Dead Space (2008)":                            "dead space",
		"Game [FitGirl Repack]":                        "game",
		"STASIS BONE TOTEM":                            "stasis bone totem",
		"The Talos Principle 2: Road to Elysium":       "the talos principle 2 road to elysium",
		"Some Game v1.0.10.0":                          "some game",
		"Fallout 4 Game of the Year Edition":           "fallout 4",
		"Ori and the Blind Forest: Definitive Edition": "ori and the blind forest",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// Score ranks one store item against the normalized candidate: exact
// normalized match dominates; token-set near-equality and PC platform add;
// edition mismatches subtract; titles of 4 chars or fewer only ever match
// exactly.
func TestScore(t *testing.T) {
	cases := []struct {
		cand string
		item string
		pc   bool
		want int
	}{
		{"dead space", "Dead Space", true, 110},                                          // exact + PC
		{"dead space", "Dead Space", false, 100},                                         // exact, no PC bonus
		{"anno 1404 gold", "Anno 1404 Gold", true, 110},                                  // exact normalized
		{"the talos principle 2 road to elysium", "The Talos Principle 2", true, 0 + 10}, // subset ≠ near-equal (strict jaccard)
		{"dead space", "Dead Space (2008)", true, 110},                                   // year tag stripped from item
		{"prey", "Prey", true, 110},                                                      // short title, exact ok
		{"doom", "Doom 3", true, 10},                                                     // short title: no fuzzy credit
		{"frostpunk", "Frostpunk 2", true, 10},                                           // strict jaccard rejects sequel bait
		{"b1", "Black Myth: Wukong", true, 10},                                           // no shared tokens
		{"samorost2", "Samorost 2", true, 110},                                           // letter→digit boundary is not a mismatch
		{"cyberpunk 2077", "Cyberpunk 2077", true, 110},                                  // digit tails still match exactly
		{"alan wake 2", "Alan Wake II", true, 110},                                       // roman numerals are the same number
		{"final fantasy xvi", "Final Fantasy XVI", true, 110},                            // multi-letter romans too
		{"doom 2", "Doom", true, 10},                                                     // different numbers stay different
	}
	for _, tc := range cases {
		if got := Score(tc.cand, tc.item, tc.pc); got != tc.want {
			t.Errorf("Score(%q, %q, %v) = %d, want %d", tc.cand, tc.item, tc.pc, got, tc.want)
		}
	}
}

// Accept encodes the threshold: ≥90 alone, or ≥75 only with a second
// corroborating signal (e.g. developer == PE CompanyName).
func TestAccept(t *testing.T) {
	cases := []struct {
		score        int
		corroborated bool
		want         bool
	}{
		{110, false, true},
		{90, false, true},
		{89, true, true},
		{75, true, true},
		{75, false, false},
		{74, true, false},
	}
	for _, tc := range cases {
		if got := Accept(tc.score, tc.corroborated); got != tc.want {
			t.Errorf("Accept(%d, %v) = %v, want %v", tc.score, tc.corroborated, got, tc.want)
		}
	}
}
