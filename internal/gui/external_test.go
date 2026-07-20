package gui

import (
	"os"
	"path/filepath"
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestQuickLabelExternal: an external (PE-detected, unmanaged) install is
// adopted, not installed over — the quick-action caption must say so.
func TestQuickLabelExternal(t *testing.T) {
	ext := &ui.GameRow{Title: "Ext", Status: domain.StatusExternal}
	if got := quickLabel(ext); got != "Adopt" {
		t.Errorf("external row: quickLabel %q, want %q", got, "Adopt")
	}
	// The existing statuses must not drift.
	if got := quickLabel(&ui.GameRow{Status: domain.StatusCommitted}); got != "Uninstall" {
		t.Errorf("committed row: quickLabel %q, want Uninstall", got)
	}
	if got := quickLabel(&ui.GameRow{}); got != "Install" {
		t.Errorf("clean row: quickLabel %q, want Install", got)
	}
	t.Log("external quick action adopts the on-disk install")
}

// TestStatusLabelExternal: the status pill text for an external row is the
// derived status itself.
func TestStatusLabelExternal(t *testing.T) {
	ext := &ui.GameRow{Status: domain.StatusExternal}
	if got := statusLabel(ext); got != "external" {
		t.Errorf("statusLabel(external) = %q, want %q", got, "external")
	}
	if got := statusLabel(&ui.GameRow{}); got != "not installed" {
		t.Errorf("statusLabel(clean) = %q, want %q", got, "not installed")
	}
}

// TestStatusToneExternal: external installs render blue — present on disk
// but not manager-owned, so neither green (committed) nor gray (absent).
func TestStatusToneExternal(t *testing.T) {
	ext := &ui.GameRow{Status: domain.StatusExternal}
	if got := statusTone(ext); got != ui.ToneBlue {
		t.Errorf("statusTone(external) = %v, want ToneBlue", got)
	}
	if got := statusTone(&ui.GameRow{Status: domain.StatusCommitted}); got != ui.ToneGreen {
		t.Errorf("statusTone(committed) = %v, want ToneGreen", got)
	}
	if got := statusTone(&ui.GameRow{}); got != ui.ToneGray {
		t.Errorf("statusTone(clean) = %v, want ToneGray", got)
	}
}

// TestVersionPillsExternalMarked: the OptiScaler version pill on an external
// row carries the external marker so a scanned unmanaged install never reads
// as manager-committed — versioned or not.
func TestVersionPillsExternalMarked(t *testing.T) {
	withVer := versionPills(&ui.GameRow{Status: domain.StatusExternal, OptiScalerVersion: "0.7.0"})
	if len(withVer) == 0 || withVer[0].Label != "✦ OptiScaler 0.7.0 · external" {
		t.Fatalf("external versioned pills %v, want first pill %q", withVer, "✦ OptiScaler 0.7.0 · external")
	}
	if withVer[0].Tone != ui.ToneBlue {
		t.Errorf("external pill tone %v, want ToneBlue", withVer[0].Tone)
	}

	noVer := versionPills(&ui.GameRow{Status: domain.StatusExternal})
	if len(noVer) != 1 || noVer[0].Label != "✦ OptiScaler · external" {
		t.Errorf("external unversioned pills %v, want [%q]", noVer, "✦ OptiScaler · external")
	}
	if len(noVer) == 1 && noVer[0].Tone != ui.ToneBlue {
		t.Errorf("external unversioned pill tone %v, want ToneBlue", noVer[0].Tone)
	}

	// Committed rows keep the unmarked purple pill.
	committed := versionPills(&ui.GameRow{Status: domain.StatusCommitted, OptiScalerVersion: "0.9.4"})
	if len(committed) == 0 || committed[0].Label != "✦ OptiScaler 0.9.4" || committed[0].Tone != ui.TonePurple {
		t.Errorf("committed pills %v, want the plain purple OptiScaler pill", committed)
	}
	t.Logf("external pills: %v / %v", withVer, noVer)
}

// TestCardBadgeExternal: both badge render paths (grid card chrome and list
// row) draw the external status pill from the same source — blue per
// statusTone — and both views render external rows without blowing up.
func TestCardBadgeExternal(t *testing.T) {
	ext := &ui.GameRow{Title: "Ext", InstallDir: "/g/ext", Status: domain.StatusExternal, OptiScalerVersion: "0.7.0"}

	b, ok := statusPill(ext)
	if !ok || b.Label != "external" || b.Tone != ui.ToneBlue {
		t.Errorf("statusPill(external) = %+v, %v; want {Label: external, Tone: ToneBlue}, true", b, ok)
	}
	// Actionable rows keep their red alert pill.
	act := &ui.GameRow{Status: domain.StatusFailed, Actionable: true}
	if b, ok = statusPill(act); !ok || b.Label != string(domain.StatusFailed) || b.Tone != ui.ToneRed {
		t.Errorf("statusPill(actionable) = %+v, %v; want the red status pill", b, ok)
	}
	// Clean rows get no alert pill.
	if _, ok = statusPill(&ui.GameRow{}); ok {
		t.Error("statusPill(clean) ok=true, want false")
	}

	// Both render paths: grid cards and list rows.
	for _, mode := range []ui.ViewMode{ui.ViewGrid, ui.ViewList} {
		m := newModel(Config{})
		m.state = ui.State{
			StatusLine: "1 games",
			Mode:       mode,
			Rows:       []ui.GameRow{*ext},
		}
		out := filepath.Join(t.TempDir(), "frame.png")
		if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
			t.Fatalf("renderToPNG mode %v: %v", mode, err)
		}
		if st, _ := os.Stat(out); st == nil || st.Size() == 0 {
			t.Fatalf("empty frame in mode %v", mode)
		}
	}
	t.Log("external badge pill renders in grid and list paths")
}

// TestDetailOpenINIVisibleForExternal: the detail panel's OpenINI button is
// gated on CanOpenINI (committed || external), not on committed alone — an
// external install has a real OptiScaler.ini on disk worth editing. The
// button rect is the observability seam: non-zero when rendered.
func TestDetailOpenINIVisibleForExternal(t *testing.T) {
	t.Run("external row shows the button", func(t *testing.T) {
		sess, gameRoot := guiFakes(t)
		marker := testutil.StringInfoPE(false, map[string]string{
			"ProductName":      "OptiScaler",
			"OriginalFilename": "OptiScaler.dll",
		}, [4]uint16{0, 7, 0, 0})
		writeGUIFile(t, filepath.Join(gameRoot, "bin", "dxgi.dll"), string(marker))

		row := scanOneRow(t, sess)
		if row.Status != domain.StatusExternal {
			t.Fatalf("row status %q, want %q", row.Status, domain.StatusExternal)
		}
		if !row.CanOpenINI() {
			t.Fatalf("external row CanOpenINI() = false; the gate cannot be proven")
		}
		m := newModel(Config{Session: sess})
		sess.Select(row.InstallDir)

		// Tall window: the panel's cover art pushes the action buttons near
		// the fold, and shirei culls clipped Viewport children (zero rect).
		headlessFrames(t, 1100, 1400)
		keyFrame(KeyCodeNone, 0, m.rootView) // build
		keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
		if m.openINIRect.Size[0] == 0 || m.openINIRect.Size[1] == 0 {
			t.Errorf("OpenINI button not rendered for external row (rect %+v)", m.openINIRect)
		}
		t.Logf("external OpenINI button rect: %+v", m.openINIRect)
	})

	t.Run("clean row hides the button", func(t *testing.T) {
		sess, _ := guiFakes(t)
		row := scanOneRow(t, sess)
		if row.CanOpenINI() {
			t.Fatalf("clean row CanOpenINI() = true (status %q)", row.Status)
		}
		m := newModel(Config{Session: sess})
		sess.Select(row.InstallDir)

		headlessFrames(t, 1100, 1400)
		keyFrame(KeyCodeNone, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		if m.openINIRect.Size[0] != 0 || m.openINIRect.Size[1] != 0 {
			t.Errorf("OpenINI button rendered for clean row (rect %+v)", m.openINIRect)
		}
		t.Log("clean row renders no OpenINI button")
	})
}
