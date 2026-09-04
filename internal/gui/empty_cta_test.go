package gui

import (
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUIEmptyStateHasCTA: an empty library offers a primary call to action;
// activating the first one starts a scan (no dead end on first run).
func TestGUIEmptyStateHasCTA(t *testing.T) {
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SteamRoot = t.TempDir() // no games anywhere
	})
	m := newModel(Config{Session: sess})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "0 games"}

	headlessFrames(t, 800, 600)
	view := func() {
		Container(Attrs(Viewport), func() {
			m.emptyState()
		})
	}
	keyFrame(KeyCodeNone, 0, view) // build + register focusables
	keyFrame(KeyTab, 0, view)      // focus the first CTA
	keyFrame(KeyEnter, 0, view)    // activate it

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev := <-sess.Events():
			// Wait for the scan to SETTLE, not just start: runScan
			// persists the games cache before emitting EvScanDone, and
			// returning early would let that write race TempDir cleanup
			// ("directory not empty" marks the test failed).
			if ev.Kind == ui.EvScanStarted || ev.Kind == ui.EvScanDone || ev.Kind == ui.EvScanFailed {
				t.Logf("empty-state CTA scan event: %d", ev.Kind)
			}
			if ev.Kind == ui.EvScanDone || ev.Kind == ui.EvScanFailed {
				return
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("first empty-state CTA did not trigger a scan")
}
