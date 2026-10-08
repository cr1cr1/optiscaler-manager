package ui

import (
	"errors"
	"fmt"
	"testing"
)

// Issue 052: the session keeps an append-only ring of status lines so the
// GUI log panel can display the same console output the status bar shows.

func TestLogRingAppendsSplitsAndDedupes(t *testing.T) {
	e := newTestEnv(t)
	e.sess.setStatus("Launch requested: Game One")
	e.sess.setStatus("Launch requested: Game One") // consecutive duplicate: skipped
	e.sess.setStatus("Launch failed: boom\nstderr two\nstderr three")
	lines := e.sess.LogLines()
	want := []string{"Launch requested: Game One", "Launch failed: boom", "stderr two", "stderr three"}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Fatalf("LogLines = %q, want %q", lines, want)
	}
}

func TestLogRingCapsOldest(t *testing.T) {
	e := newTestEnv(t)
	for i := 0; i < logRingCap+5; i++ {
		e.sess.setStatus(fmt.Sprintf("line %d", i))
	}
	lines := e.sess.LogLines()
	if len(lines) != logRingCap {
		t.Fatalf("LogLines len = %d, want cap %d", len(lines), logRingCap)
	}
	if lines[0] != "line 5" {
		t.Fatalf("oldest kept line = %q, want %q", lines[0], "line 5")
	}
	last := fmt.Sprintf("line %d", logRingCap+4)
	if lines[len(lines)-1] != last {
		t.Fatalf("newest line = %q, want %q", lines[len(lines)-1], last)
	}
}

// Issue 057: game-scoped status lines carry the row title so the GUI log
// panel can prefix them in a distinct color; global lines stay gameless.

func TestLogEntriesCarryGameTitle(t *testing.T) {
	e := newTestEnv(t)
	e.sess.mu.Lock()
	e.sess.st.Rows = []GameRow{{InstallDir: e.gameRoot, Title: "Game One"}}
	e.sess.mu.Unlock()

	e.sess.opFailed(errors.New("boom"), e.gameRoot)
	e.sess.setStatus("Scanning…")

	entries := e.sess.LogEntries()
	if len(entries) != 2 {
		t.Fatalf("LogEntries len = %d, want 2 (%v)", len(entries), entries)
	}
	if entries[0].Game != "Game One" || entries[0].Line != "Failed: boom" {
		t.Errorf("game entry = %+v, want {Failed: boom, Game One}", entries[0])
	}
	if entries[1].Game != "" || entries[1].Line != "Scanning…" {
		t.Errorf("global entry = %+v, want {Scanning…, no game}", entries[1])
	}
	if lines := e.sess.LogLines(); len(lines) != 2 || lines[0] != "Failed: boom" {
		t.Errorf("LogLines derivation broken: %q", lines)
	}
}

func TestLogEntryCapKeepsTitlesAligned(t *testing.T) {
	e := newTestEnv(t)
	e.sess.mu.Lock()
	e.sess.st.Rows = []GameRow{{InstallDir: e.gameRoot, Title: "Game One"}}
	e.sess.mu.Unlock()
	for i := 0; i < logRingCap+5; i++ {
		e.sess.setStatusGame(e.gameRoot, fmt.Sprintf("line %d", i))
	}
	entries := e.sess.LogEntries()
	if len(entries) != logRingCap {
		t.Fatalf("len = %d, want %d", len(entries), logRingCap)
	}
	if entries[0].Line != "line 5" || entries[0].Game != "Game One" {
		t.Fatalf("eviction misaligned titles: %+v", entries[0])
	}
}
