package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// TestSessionUmuGameIDFor: the GAMEID/STORE resolution precedence for
// umu launches (issue 064): per-game override → umu-<SteamAppID> when
// identified → umu-database lookup (Heroic store codenames) →
// "umu-default" with empty store. Lookups degrade, never block.
func TestSessionUmuGameIDFor(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.Scan(t.Context())
	waitScanDone(t, e.sess)
	dir := e.gameRoot

	lookups := map[string]string{}
	e.sess.deps.UmuGameIDLookup = func(_ context.Context, store, codename string) (string, error) {
		id, ok := lookups[store+":"+codename]
		if !ok {
			return "", errors.New("no entry")
		}
		return id, nil
	}

	base := GameRow{Title: "T", InstallDir: dir, Store: domain.StoreManual}

	// Unidentified manual game: the umu-default pair, no store.
	id, store := e.sess.UmuGameIDFor(t.Context(), base)
	if id != "umu-default" || store != "" {
		t.Errorf("manual unidentified = (%q, %q), want (umu-default, \"\")", id, store)
	}

	// Identified via Steam: the ID rule — umu-<SteamAppID> even when
	// owned elsewhere; store stays empty for a manual copy.
	row := base
	row.SteamAppID = "1174180"
	id, store = e.sess.UmuGameIDFor(t.Context(), row)
	if id != "umu-1174180" || store != "" {
		t.Errorf("identified = (%q, %q), want (umu-1174180, \"\")", id, store)
	}

	// Heroic Epic row without a SteamAppID: the DB lookup maps the
	// (egs, AppName) codename.
	lookups["egs:Heather"] = "umu-1174180"
	epic := GameRow{Title: "RDR2", InstallDir: dir, Store: domain.StoreEpic, AppID: "Heather", AppName: "Heather"}
	id, store = e.sess.UmuGameIDFor(t.Context(), epic)
	if id != "umu-1174180" || store != "egs" {
		t.Errorf("epic db hit = (%q, %q), want (umu-1174180, egs)", id, store)
	}

	// Heroic GOG row: (gog, AppID) codename; a lookup failure degrades
	// to umu-default with the row's store still set.
	gog := GameRow{Title: "G", InstallDir: dir, Store: domain.StoreGOG, AppID: "1422357892"}
	id, store = e.sess.UmuGameIDFor(t.Context(), gog)
	if id != "umu-default" || store != "gog" {
		t.Errorf("gog db miss = (%q, %q), want (umu-default, gog)", id, store)
	}

	// Per-game override wins over everything (JSON-edited, the
	// SteamGridDBKey precedent).
	e.sess.updateSettings(func(st *settings.Settings) bool {
		st.GameIDOverrides = map[string]string{canonicalDir(dir): "umu-custom-1"}
		return true
	}, "", "")
	id, _ = e.sess.UmuGameIDFor(t.Context(), row)
	if id != "umu-custom-1" {
		t.Errorf("override = %q, want umu-custom-1", id)
	}
}

// TestUmuStoreID: the domain store → umu store id mapping.
func TestUmuStoreID(t *testing.T) {
	cases := map[domain.Store]string{
		domain.StoreEpic:   "egs",
		domain.StoreGOG:    "gog",
		domain.StoreSteam:  "steam",
		domain.StoreManual: "",
	}
	for st, want := range cases {
		if got := umuStoreID(st); got != want {
			t.Errorf("umuStoreID(%v) = %q, want %q", st, got, want)
		}
	}
}
