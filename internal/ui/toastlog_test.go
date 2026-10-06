package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// captureLog swaps the global zerolog output for a buffer (the "log
// console": stderr for CLI/GUI, tui.log for the TUI) and restores it on
// cleanup. Package tests run sequentially, so the swap is race-free.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := log.Logger
	prevGlobal := zerolog.GlobalLevel()
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		log.Logger = prev
		zerolog.SetGlobalLevel(prevGlobal)
	})
	return &buf
}

// Every user-facing warning/error (warn toast) MUST also land in the log
// console; info toasts stay out of it.
func TestWarnToastsReachTheLogConsole(t *testing.T) {
	e := newTestEnv(t)
	buf := captureLog(t)

	e.sess.toast("disk full", true)
	if out := buf.String(); !strings.Contains(out, "disk full") {
		t.Errorf("warn toast missing from the log console: %q", out)
	}
	if out := buf.String(); !strings.Contains(out, `"level":"warn"`) {
		t.Errorf("toast log entry is not at warn level: %q", out)
	}

	buf.Reset()
	e.sess.toast("Installed Game One", false)
	if out := buf.String(); strings.Contains(out, "Installed Game One") {
		t.Errorf("info toast leaked into the log console: %q", out)
	}
}

// The op-failure funnel lands the underlying error text in the log
// console through the same path.
func TestOpFailedReachesTheLogConsole(t *testing.T) {
	e := newTestEnv(t)
	buf := captureLog(t)

	e.sess.opFailed(errors.New("store exploded"), "")
	if out := buf.String(); !strings.Contains(out, "store exploded") {
		t.Errorf("op failure missing from the log console: %q", out)
	}
}
