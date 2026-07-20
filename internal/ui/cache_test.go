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
