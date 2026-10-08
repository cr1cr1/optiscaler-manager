package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// A per-game rescan re-runs the scan pipeline for one game only —
// rediscovery, online identification, and the cover rebind — and settles
// the refreshed row like a scan would (sorted, persisted, toasted). The
// user fixes nothing by hand: the same heuristics a global scan runs are
// applied to just this game (issues 030/033/034 made rescans the fix
// vehicle; a full-library rescan per game is too slow).
func TestRescanGameRefreshesManualGame(t *testing.T) {
	f := newIdentifyFixture(t)
	stale := settings.Defaults()
	stale.OnlineLookups = false // the stale baseline: an offline library
	f.sess.deps.Settings = stale
	cf := newCoverFake(t)
	f.sess.deps.Covers = cf.covers(t)

	root := t.TempDir()
	dir := filepath.Join(root, "cyberpunk2077")
	writeUIFile(t, filepath.Join(dir, "cyberpunk2077.exe"), "MZGAME")
	f.sess.deps.Settings.ExtraDirs = []string{root}
	f.search["cyberpunk2077"] = `{"total":1,"items":[{"id":1091500,"name":"Cyberpunk 2077","type":"app","platforms":{"windows":true}}]}`
	f.appdetails["1091500"] = [2]string{"Cyberpunk 2077", "CD PROJEKT RED"}

	// Baseline: the offline scan rows the game from its folder, never
	// identified (no appid, folder title).
	st := scanAndWait(t, f.sess)
	var base *GameRow
	for i := range st.Rows {
		if st.Rows[i].InstallDir == canonicalDir(dir) {
			base = &st.Rows[i]
		}
	}
	if base == nil {
		t.Fatal("baseline row missing")
	}
	if base.SteamAppID != "" {
		t.Fatalf("baseline already identified: %+v", base)
	}

	// Online lookups on (the user's real setting); the rescan refreshes
	// this one game through the full pipeline.
	f.sess.deps.Settings.OnlineLookups = true
	f.sess.RescanGame(dir)
	waitEventText(t, f.sess, EvScanDone, "rescanned Cyberpunk 2077")

	row := f.sess.findRow(canonicalDir(dir))
	if row == nil {
		t.Fatal("row vanished after rescan")
	}
	if row.Title != "Cyberpunk 2077" || row.SteamAppID != "1091500" {
		t.Errorf("row = title %q appid %q, want the identified canonical pair", row.Title, row.SteamAppID)
	}
	if !strings.HasSuffix(row.CoverPath, "1091500.img") {
		t.Errorf("CoverPath = %q, want art for the identified appid", row.CoverPath)
	}
	// The refreshed row persists like a scan settle would.
	cached := loadGamesCache(f.sess.deps.SettingsRoot, f.sess.deps.GOOS)
	persisted := false
	for _, c := range cached {
		if c.InstallDir == row.InstallDir && c.SteamAppID == "1091500" {
			persisted = true
		}
	}
	if !persisted {
		t.Error("games cache does not hold the rescanned row")
	}
	t.Logf("rescan refreshed: %q (%s), cover %s", row.Title, row.SteamAppID, row.CoverPath)
}

// A game that no longer resolves anywhere (deleted from disk) keeps its
// row — pruning is the global scan's job — and the rescan says so.
func TestRescanGameVanishedGameKeepsRow(t *testing.T) {
	f := newIdentifyFixture(t)
	offline := settings.Defaults()
	offline.OnlineLookups = false
	f.sess.deps.Settings = offline

	root := t.TempDir()
	dir := filepath.Join(root, "ghostgame")
	writeUIFile(t, filepath.Join(dir, "ghostgame.exe"), "MZGAME")
	f.sess.deps.Settings.ExtraDirs = []string{root}
	st := scanAndWait(t, f.sess)
	key := canonicalDir(dir)
	oldTitle := ""
	for _, r := range st.Rows {
		if r.InstallDir == key {
			oldTitle = r.Title
		}
	}
	if oldTitle == "" {
		t.Fatal("baseline row missing")
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	f.sess.RescanGame(dir)

	deadline := time.Now().Add(15 * time.Second)
	seen := ""
	for time.Now().Before(deadline) {
		for _, to := range f.sess.Snapshot().Toasts {
			if strings.Contains(to.Text, "no longer found") {
				seen = to.Text
			}
		}
		if seen != "" {
			break
		}
		select {
		case <-f.sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if seen == "" {
		t.Fatal("no 'no longer found' toast after rescanning a deleted game")
	}
	if !toastsWarn(f.sess.Snapshot().Toasts, seen) {
		t.Errorf("toast %q not marked as a warning", seen)
	}
	row := f.sess.findRow(key)
	if row == nil {
		t.Error("deleted game's row was pruned by a per-game rescan; pruning is the global scan's job")
	} else if row.Title != oldTitle {
		t.Errorf("row title = %q, want the untouched %q", row.Title, oldTitle)
	}
	t.Logf("vanished game kept its row, toast: %q", seen)
}

func toastsWarn(toasts []Toast, text string) bool {
	for _, to := range toasts {
		if to.Text == text {
			return to.Warn
		}
	}
	return false
}
