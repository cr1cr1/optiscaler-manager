package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// writeGamesCache seeds the games-list cache at root with rows.
func writeGamesCache(t *testing.T, root string, rows []GameRow) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(gamesCache{Version: cacheSchemaVersion, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "games.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// readGamesCache loads and parses the games-list cache at root.
func readGamesCache(t *testing.T, root string) gamesCache {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "games.json"))
	if err != nil {
		t.Fatalf("read games cache: %v", err)
	}
	var c gamesCache
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse games cache: %v", err)
	}
	return c
}

// assertNoScanEvents fails when a scan event surfaces within the grace
// window; used to prove cache paths never masquerade as scans.
func assertNoScanEvents(t *testing.T, s *Session) {
	t.Helper()
	select {
	case ev := <-s.Events():
		if ev.Kind == EvScanStarted || ev.Kind == EvScanDone {
			t.Fatalf("unexpected scan event: %v %q", ev.Kind, ev.Text)
		}
		t.Logf("non-scan event (ok): %v %q", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestStart_StaleSchemaCacheFallsThroughToScan: a games.json written at the
// v0.6 schema (version 1 — rows predate the v0.7 container self-row
// semantics) is invalidated by the schema bump: it loads as empty and Start
// falls through to a real scan rather than resurrecting stale rows.
func TestStart_StaleSchemaCacheFallsThroughToScan(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	stale := []GameRow{{
		Title:      "StaleContainer",
		AppID:      "custom_StaleContainer",
		InstallDir: filepath.Join(t.TempDir(), "StaleContainer"),
	}}
	data, err := json.Marshal(gamesCache{Version: 1, Rows: stale})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.sess.deps.SettingsRoot, "games.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())
	waitEvent(t, e.sess, EvScanDone) // stale cache rejected: a real scan ran

	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == stale[0].InstallDir {
			t.Fatalf("stale v1 row survived warm boot: %+v", r)
		}
	}
	t.Log("v1 cache invalidated by schema bump; Start fell through to scan")
}

// TestLoadGamesCacheStripsProtonTierOffLinux: a games.json carried over from
// a linux profile (or written before the gate) holds ProtonDB tiers that are
// meaningless off-linux. Loading strips them in place — no schema bump, the
// cache self-heals — while a linux load preserves them.
func TestLoadGamesCacheStripsProtonTierOffLinux(t *testing.T) {
	root := t.TempDir()
	writeGamesCache(t, root, []GameRow{
		{Title: "Tiered", AppID: "100", InstallDir: "/games/tiered", ProtonTier: "gold"},
	})

	offLinux := loadGamesCache(root, "darwin")
	if len(offLinux) != 1 {
		t.Fatalf("off-linux rows = %d, want 1", len(offLinux))
	}
	if offLinux[0].ProtonTier != "" {
		t.Errorf("off-linux ProtonTier = %q, want empty (stripped at load)", offLinux[0].ProtonTier)
	}

	onLinux := loadGamesCache(root, "linux")
	if len(onLinux) != 1 {
		t.Fatalf("linux rows = %d, want 1", len(onLinux))
	}
	if onLinux[0].ProtonTier != "gold" {
		t.Errorf("linux ProtonTier = %q, want %q (preserved)", onLinux[0].ProtonTier, "gold")
	}
	t.Log("cache load: tier stripped on darwin, preserved on linux")
}

// TestScanPersistsCache: a completed scan writes games.json whose rows match
// the session rows on the stable fields (InstallDir, Title, Status).
func TestScanPersistsCache(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	c := readGamesCache(t, e.sess.deps.SettingsRoot)
	st := e.sess.Snapshot()
	if len(c.Rows) != len(st.Rows) || len(c.Rows) == 0 {
		t.Fatalf("cached rows %d, session rows %d", len(c.Rows), len(st.Rows))
	}
	for i, r := range st.Rows {
		cr := c.Rows[i]
		if cr.InstallDir != r.InstallDir || cr.Title != r.Title || cr.Status != r.Status {
			t.Errorf("cached row %d = %+v, want InstallDir/Title/Status of %+v", i, cr, r)
		}
	}
	t.Logf("cache persisted %d rows after scan", len(c.Rows))
}

// TestAddDirectoryPersistsCache: adding a manual directory rewrites the
// cache so the new row survives a restart.
func TestAddDirectoryPersistsCache(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "CacheGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.AddDirectory(custom)
	waitEventText(t, e.sess, EvScanDone, "directory added")

	c := readGamesCache(t, e.sess.deps.SettingsRoot)
	found := false
	for _, r := range c.Rows {
		if r.InstallDir == custom && r.Title == "CacheGame" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added dir missing from cache: %+v", c.Rows)
	}
	t.Log("added directory persisted to cache")
}

// TestOpSettlePersistsCacheStatus: when an install settles, the cache
// reflects the row's new status — and the cache write itself emits no scan
// events (it is not a rescan).
func TestOpSettlePersistsCacheStatus(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	c := readGamesCache(t, e.sess.deps.SettingsRoot)
	found := false
	for _, r := range c.Rows {
		if r.InstallDir == e.gameRoot {
			found = true
			if r.Status != domain.StatusCommitted {
				t.Fatalf("cached status %q, want committed", r.Status)
			}
		}
	}
	if !found {
		t.Fatalf("game row missing from cache: %+v", c.Rows)
	}
	assertNoScanEvents(t, e.sess)
	t.Log("op settle persisted status without scan events")
}

// TestStartWithCacheSkipsScan: a warm cache boots the library straight from
// disk — cached titles (which no fixture scan would produce), a "(cached)"
// status line, and no scan activity at all.
func TestStartWithCacheSkipsScan(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	writeGamesCache(t, e.sess.deps.SettingsRoot, []GameRow{
		{Title: "Cached Alpha", AppID: "ca", InstallDir: "/cache/alpha"},
		{Title: "Cached Bravo", AppID: "cb", InstallDir: "/cache/bravo"},
		{Title: "Cached Charlie", AppID: "cc", InstallDir: "/cache/charlie"},
	})

	e.sess.Start(context.Background())

	st := e.sess.Snapshot()
	if len(st.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 cached rows", len(st.Rows))
	}
	for i, want := range []string{"Cached Alpha", "Cached Bravo", "Cached Charlie"} {
		if st.Rows[i].Title != want {
			t.Errorf("row %d title %q, want cached %q (scan leaked into Start?)", i, st.Rows[i].Title, want)
		}
	}
	if st.StatusLine != "3 games (cached)" {
		t.Errorf("status line %q, want %q", st.StatusLine, "3 games (cached)")
	}
	if st.Busy != "" {
		t.Errorf("busy %q after cached start, want idle", st.Busy)
	}
	assertNoScanEvents(t, e.sess)
	t.Logf("cached start: %q, no scan events", st.StatusLine)
}

// TestStartWithoutCacheScans: a cold profile (no games.json) falls through
// to the real scan — the first-run contract is preserved.
func TestStartWithoutCacheScans(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.Start(context.Background())
	waitEvent(t, e.sess, EvScanStarted)
	ev := waitEvent(t, e.sess, EvScanDone)

	st := e.sess.Snapshot()
	if len(st.Rows) != 1 || st.Rows[0].Title != "Game One" {
		t.Fatalf("rows = %+v, want the fixture game", st.Rows)
	}
	t.Logf("cold start scanned: %q", ev.Text)
}

// TestCacheHydrateReconcilesStatusFromManifests: install status is
// reconciled from store manifests at hydration (cheap — no PE parsing, no
// reclassification), so a stale cached status self-heals.
func TestCacheHydrateReconcilesStatusFromManifests(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	gameRoot := canonicalDir(e.gameRoot)
	binDir := canonicalDir(e.bin)
	writeGamesCache(t, e.sess.deps.SettingsRoot, []GameRow{
		{Title: "Game One", AppID: "100", InstallDir: gameRoot, InjectionDir: binDir},
	})
	m := &domain.Manifest{
		ID:            domain.ManifestID(binDir),
		SchemaVersion: domain.SchemaVersion,
		Status:        domain.StatusCommitted,
		GameRoot:      gameRoot,
		InstallDir:    binDir,
	}
	if err := e.sess.deps.Store.Save(m); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())

	st := e.sess.Snapshot()
	if len(st.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 cached row", len(st.Rows))
	}
	if st.Rows[0].Status != domain.StatusCommitted {
		t.Fatalf("hydrated status %q, want committed from manifest", st.Rows[0].Status)
	}
	assertNoScanEvents(t, e.sess)
	t.Log("stale cached status reconciled from manifest")
}

// TestCacheCorruptOrMissingYieldsEmpty: a corrupt cache is not an error —
// Start surfaces nothing, hydrates zero rows, and falls through to a scan.
func TestCacheCorruptOrMissingYieldsEmpty(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	if err := os.WriteFile(filepath.Join(e.sess.deps.SettingsRoot, "games.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())
	waitEvent(t, e.sess, EvScanStarted)
	ev := waitEvent(t, e.sess, EvScanDone)

	st := e.sess.Snapshot()
	if len(st.Rows) != 1 || st.Rows[0].Title != "Game One" {
		t.Fatalf("rows = %+v, want the fixture game from the fallback scan", st.Rows)
	}
	for _, toast := range st.Toasts {
		if toast.Warn {
			t.Errorf("warning toast from corrupt cache: %q", toast.Text)
		}
	}
	t.Logf("corrupt cache fell through to scan: %q", ev.Text)
}

// drainEvents empties the event buffer so the next waitEvent observes only
// events emitted after this call.
func drainEvents(s *Session) {
	for {
		select {
		case <-s.Events():
		default:
			return
		}
	}
}

// TestRemoveDirectoryRemovesRowsSettingsAndCache: removing a manual root
// drops its own row AND nested games scanned under it, persists the shorter
// ExtraDirs, rewrites the cache, and settles with an EvScanDone event.
func TestRemoveDirectoryRemovesRowsSettingsAndCache(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	d1 := filepath.Join(t.TempDir(), "D1")
	writeUIFile(t, filepath.Join(d1, "game.exe"), "GAME")
	writeUIFile(t, filepath.Join(d1, "sub", "sub.exe"), "GAME")
	d2 := filepath.Join(t.TempDir(), "D2")
	writeUIFile(t, filepath.Join(d2, "game.exe"), "GAME")

	e.sess.AddDirectory(d1)
	e.sess.AddDirectory(d2)
	waitEventText(t, e.sess, EvScanDone, "directory added")
	waitEventText(t, e.sess, EvScanDone, "directory added")
	drainEvents(e.sess)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	c1, c2 := canonicalDir(d1), canonicalDir(d2)
	nested := filepath.Join(c1, "sub")
	has := func(dir string) bool {
		for _, r := range e.sess.Snapshot().Rows {
			if r.InstallDir == dir {
				return true
			}
		}
		return false
	}
	if !has(c1) || !has(nested) || !has(c2) {
		t.Fatalf("precondition rows missing: d1=%v nested=%v d2=%v", has(c1), has(nested), has(c2))
	}

	e.sess.RemoveDirectory(d1)

	loaded, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ExtraDirs) != 1 || loaded.ExtraDirs[0] != c2 {
		t.Fatalf("ExtraDirs after removal: %v, want [%s]", loaded.ExtraDirs, c2)
	}
	st := e.sess.Snapshot()
	if has(c1) || has(nested) {
		t.Fatalf("rows for removed dir survived: %+v", st.Rows)
	}
	if !has(c2) {
		t.Fatalf("unrelated dir row lost: %+v", st.Rows)
	}
	c := readGamesCache(t, e.sess.deps.SettingsRoot)
	if len(c.Rows) != len(st.Rows) {
		t.Fatalf("cache rows %d, session rows %d", len(c.Rows), len(st.Rows))
	}
	for _, r := range c.Rows {
		if r.InstallDir == c1 || r.InstallDir == nested {
			t.Fatalf("removed dir still cached: %+v", r)
		}
	}
	ev := waitEvent(t, e.sess, EvScanDone)
	t.Logf("directory removed: %q, ExtraDirs=%v, rows=%d", ev.Text, loaded.ExtraDirs, len(st.Rows))
}

// TestRemoveDirectoryAbsentIsNoOp: removing a dir that was never added
// writes nothing, emits nothing, and panics nowhere.
func TestRemoveDirectoryAbsentIsNoOp(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.RemoveDirectory(filepath.Join(t.TempDir(), "never-added"))

	if _, err := os.Stat(filepath.Join(e.sess.deps.SettingsRoot, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("settings written for an absent directory")
	}
	if got := len(e.sess.Snapshot().Rows); got != 0 {
		t.Fatalf("rows changed by no-op removal: %d", got)
	}
	select {
	case ev := <-e.sess.Events():
		t.Fatalf("event from no-op removal: %v %q", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
	t.Log("absent directory removal is a silent no-op")
}

// TestSetSortOrdersVisibleRows: SortName orders alphabetically, SortDefault
// restores actionable-first, and an out-of-range mode is SortDefault.
func TestSetSortOrdersVisibleRows(t *testing.T) {
	now := time.Now()
	s := NewSession(Deps{})
	s.st.Rows = []GameRow{
		{Title: "Bravo", Status: domain.StatusFailed, Actionable: true, ModTime: now},
		{Title: "Alpha", ModTime: now},
		{Title: "Charlie", ModTime: now},
	}
	titles := func() []string {
		var out []string
		for _, r := range s.VisibleRows() {
			out = append(out, r.Title)
		}
		return out
	}
	want := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("visible %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("visible %v, want %v", got, want)
			}
		}
	}

	want(titles(), "Bravo", "Alpha", "Charlie") // actionable first
	s.SetSort(SortName)
	want(titles(), "Alpha", "Bravo", "Charlie")
	s.SetSort(SortDefault)
	want(titles(), "Bravo", "Alpha", "Charlie")
	s.SetSort(SortName)
	s.SetSort(SortMode(99)) // invalid resets to default
	want(titles(), "Bravo", "Alpha", "Charlie")
	t.Log("sort modes: name alphabetical, default actionable-first, invalid = default")
}

// TestStart_PreV4CacheFallsThroughToScan: games.json files written before
// schema v5 carry rows produced by older identification semantics —
// v0.7's phantom container rows (v2), v0.7.1's platform/junk rows (v3),
// and v0.7.2 rows without identification sources (v4). Every older
// schema loads as empty and Start falls through to a real scan.
func TestStart_PreV4CacheFallsThroughToScan(t *testing.T) {
	for _, version := range []int{1, 2, 3, 4} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			e := newTestEnv(t)
			e.sess.deps.SettingsRoot = t.TempDir()
			stale := []GameRow{{
				Title:      "Game A Real Title",
				AppID:      "manual_Steam",
				InstallDir: filepath.Join(t.TempDir(), "Steam"),
			}}
			data, err := json.Marshal(gamesCache{Version: version, Rows: stale})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(e.sess.deps.SettingsRoot, "games.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}

			e.sess.Start(context.Background())
			waitEvent(t, e.sess, EvScanDone) // stale cache rejected: a real scan ran

			for _, r := range e.sess.Snapshot().Rows {
				if r.InstallDir == stale[0].InstallDir {
					t.Fatalf("stale v%d container row survived warm boot: %+v", version, r)
				}
			}
		})
	}
	t.Log("pre-v4 caches invalidated; Start fell through to scan")
}
