package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestTierBadge_RendersWhenPresent: the ProtonDB tier pill maps every known
// tier to a tone, renders on the card when ProtonTier is set, and stays
// hidden when it is empty or unknown.
func TestTierBadge_RendersWhenPresent(t *testing.T) {
	for _, tier := range []string{"platinum", "gold", "silver", "bronze", "borked", "pending"} {
		if _, ok := tierPillStyle(tier); !ok {
			t.Errorf("tierPillStyle(%q) unmapped, want a tone for every ProtonDB tier", tier)
		}
	}
	for _, bogus := range []string{"", "unknown", "GOLD"} {
		if _, ok := tierPillStyle(bogus); ok {
			t.Errorf("tierPillStyle(%q) mapped, want ok=false for empty/unknown tiers", bogus)
		}
	}

	m := newModel(Config{})
	row := ui.GameRow{Title: "Tier Game", InstallDir: "/games/tier", ProtonTier: "gold"}

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := func() {
		Container(Attrs(Viewport), func() {
			m.fitCards(400)
			m.gameCard(row)
		})
	}
	keyFrame(KeyCodeNone, 0, view) // build
	keyFrame(KeyCodeNone, 0, view) // capture rects from the previous frame
	if m.tierPillRect.Size[0] == 0 || m.tierPillRect.Size[1] == 0 {
		t.Errorf("tier pill not rendered for ProtonTier %q: %+v", row.ProtonTier, m.tierPillRect)
	}

	row.ProtonTier = ""
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.tierPillRect.Size[0] != 0 {
		t.Errorf("tier pill rendered with empty ProtonTier: %+v", m.tierPillRect)
	}
	t.Logf("tier pill renders for gold at %+v, hidden when empty", m.tierPillRect)
}
