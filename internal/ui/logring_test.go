package ui

import (
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
