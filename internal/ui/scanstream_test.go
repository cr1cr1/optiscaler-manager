package ui

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestScanStream_RowsVisibleBeforeScanSettles: with the scan parked on its
// first cover fetch (the covers phase runs after discovery), the
// discovered game's card is already in live state — streamed during
// discovery, cover still unbound. After the gate releases, the settle
// binds the cover placeholder.
func TestScanStream_RowsVisibleBeforeScanSettles(t *testing.T) {
	g := newGatedCoversEnv(t)
	e := g.testEnv
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.Scan(context.Background())
	select {
	case <-g.blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("scan never reached its first cover fetch")
	}

	rows := e.sess.Snapshot().Rows
	var midErr error
	switch {
	case len(rows) != 1 || rows[0].Title != "Game One":
		midErr = fmt.Errorf("mid-scan rows = %+v, want the one discovered game already rendered", rows)
	case rows[0].CoverPath != "":
		midErr = fmt.Errorf("mid-scan CoverPath = %q, want \"\" (cards render before their art resolves)", rows[0].CoverPath)
	}
	close(g.gate)
	if midErr != nil {
		t.Fatal(midErr)
	}

	waitEvent(t, e.sess, EvScanDone)
	rows = e.sess.Snapshot().Rows
	if len(rows) != 1 || rows[0].CoverPath == "" {
		t.Fatalf("settled rows = %+v, want one row with a bound cover (placeholder on miss)", rows)
	}
	t.Log("card rendered mid-scan without art; cover bound at settle")
}

// TestScanStream_ExistingRowRefreshedInPlace: a row already in state (warm
// cache, previous scan) is refreshed in place when the scan re-discovers
// its game — not duplicated, not cleared-then-rebuilt.
func TestScanStream_ExistingRowRefreshedInPlace(t *testing.T) {
	g := newGatedCoversEnv(t)
	e := g.testEnv
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.mu.Lock()
	e.sess.st.Rows = []GameRow{{Title: "Stale Title", InstallDir: canonicalDir(e.gameRoot)}}
	e.sess.st.StatusLine = "1 games (cached)"
	e.sess.mu.Unlock()

	e.sess.Scan(context.Background())
	select {
	case <-g.blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("scan never reached its first cover fetch")
	}

	rows := e.sess.Snapshot().Rows
	var midErr error
	switch {
	case len(rows) != 1:
		midErr = fmt.Errorf("mid-scan rows = %+v, want the stale row refreshed in place (no duplicate)", rows)
	case rows[0].Title != "Game One":
		midErr = fmt.Errorf("mid-scan title = %q, want %q (stale row refreshed)", rows[0].Title, "Game One")
	}
	close(g.gate)
	if midErr != nil {
		t.Fatal(midErr)
	}

	waitEvent(t, e.sess, EvScanDone)
	if rows := e.sess.Snapshot().Rows; len(rows) != 1 || rows[0].Title != "Game One" {
		t.Fatalf("settled rows = %+v, want the single refreshed row", rows)
	}
	t.Log("pre-existing row refreshed in place mid-scan, no duplicate")
}
