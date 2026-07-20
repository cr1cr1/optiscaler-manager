package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
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
