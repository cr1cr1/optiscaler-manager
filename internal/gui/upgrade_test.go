package gui

import (
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestQuickLabelShowsUpgrade: an upgrade-eligible row replaces the toggle
// caption with the upgrade offer (cards and the detail panel share
// quickLabel, so both light up); ineligible rows keep the existing labels.
func TestQuickLabelShowsUpgrade(t *testing.T) {
	eligibleCommitted := &ui.GameRow{Title: "A", Status: domain.StatusCommitted, UpgradeAvailable: true, UpgradeTarget: "0.10.0"}
	if got := quickLabel(eligibleCommitted); got != "Upgrade to 0.10.0" {
		t.Errorf("eligible committed row: %q, want %q", got, "Upgrade to 0.10.0")
	}
	eligibleExternal := &ui.GameRow{Title: "B", Status: domain.StatusExternal, UpgradeAvailable: true, UpgradeTarget: "0.10.0"}
	if got := quickLabel(eligibleExternal); got != "Upgrade to 0.10.0" {
		t.Errorf("eligible external row: %q, want %q", got, "Upgrade to 0.10.0")
	}

	// Ineligible rows keep the toggle captions.
	committed := &ui.GameRow{Title: "C", Status: domain.StatusCommitted}
	if got := quickLabel(committed); got != "Uninstall" {
		t.Errorf("plain committed row: %q, want Uninstall", got)
	}
	external := &ui.GameRow{Title: "D", Status: domain.StatusExternal}
	if got := quickLabel(external); got != "Adopt" {
		t.Errorf("plain external row: %q, want Adopt", got)
	}
	clean := &ui.GameRow{Title: "E"}
	if got := quickLabel(clean); got != "Install" {
		t.Errorf("clean row: %q, want Install", got)
	}

	// Defensive: an offer flag without a target falls through to the toggle.
	noTarget := &ui.GameRow{Title: "F", Status: domain.StatusCommitted, UpgradeAvailable: true}
	if got := quickLabel(noTarget); got != "Uninstall" {
		t.Errorf("offer without target: %q, want Uninstall (toggle fallback)", got)
	}
	t.Log("eligible rows show the upgrade offer; every other row keeps its caption")
}
