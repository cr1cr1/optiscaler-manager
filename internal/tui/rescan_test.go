package tui

import (
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 035: the TUI detail view offers the same per-game rescan as the
// GUI detail pane's Rescan button — one key, same session method.
func TestDetailActionsIncludeRescan(t *testing.T) {
	out := sgrRE.ReplaceAllString(detailModelFor(t, ui.GameRow{Title: "G", InstallDir: "/g/one"}).detailView(100, 40), "")
	if !strings.Contains(out, "  R  rescan game") {
		t.Errorf("detail actions missing the rescan line:\n%s", out)
	}
}

// 'R' on the detail screen runs Session.RescanGame for the detail row: the
// fake steam library's game settles a "rescanned" toast.
func TestDetailKeyRescan(t *testing.T) {
	e := newTestEnv(t, nil)
	m := Model{sess: e.sess, screen: screenDetail, detailDir: e.gameRoot}

	m.detailKey(runeKey('R'))

	pollUntil(t, "rescan toast", func() bool {
		for _, to := range e.sess.Snapshot().Toasts {
			if strings.HasPrefix(to.Text, "rescanned ") {
				return true
			}
		}
		return false
	})
	t.Logf("detail key settled: %v", e.sess.Snapshot().Toasts)
}
