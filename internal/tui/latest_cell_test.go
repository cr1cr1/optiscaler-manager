package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestTUIStagedLatestCellRendersFully pins the staged-candidate rendering:
// the games-table version cell shows a designed short label ("→ Latest")
// instead of a mid-tag truncation artifact ("→ Latest (v0.9"), which read
// as version v0.9; the detail screen's action line keeps the full
// "Latest (tag)" label (it is not width-capped).
func TestTUIStagedLatestCellRendersFully(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Settings{DefaultVersion: "v0.10.0-test", OnlineLookups: true}
	})
	installed := installCommitted(t, e)
	if installed != "v0.10.0-test" {
		t.Fatalf("installed version = %q, want v0.10.0-test", installed)
	}
	pollUntil(t, "latest known", func() bool {
		return e.sess.LatestKnown() == "v0.9.4-test"
	})
	tm := startTUI(t, e.sess)
	waitFrame(t, tm, "Game One")

	tm.Type("v")
	var gamesFrame string
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		gamesFrame = string(b)
		return bytes.Contains(b, []byte("→ Latest"))
	}, teatest.WithDuration(15*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
	if strings.Contains(gamesFrame, "→ Latest (v0") {
		t.Errorf("games cell shows a mid-tag truncation fragment of the Latest label:\n%s", gamesFrame)
	}

	// Esc cancels; the detail screen renders the full Latest label.
	sendKey(tm, tea.KeyEsc)
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "AppID")
	tm.Type("v")
	waitFrame(t, tm, "Latest (v0.9.4-test)")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("detail frame with the full Latest label:\n%s", frame)
}
