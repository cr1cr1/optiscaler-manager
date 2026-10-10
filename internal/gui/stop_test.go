package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// seedGUIGamesCache writes a games.json the session loads on Start
// (internal/tui's seedGamesCache pattern; the schema version must
// track ui's cacheSchemaVersion).
func seedGUIGamesCache(t *testing.T, root string, rows []ui.GameRow) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Version int          `json:"version"`
		Rows    []ui.GameRow `json:"rows"`
	}{Version: 6, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "games.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGUIDetailLaunchLogButton: the detail panel always shows the
// Launch log affordance (issue 070); with no logs it toasts instead
// of erroring.
func TestGUIDetailLaunchLogButton(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.LogsDir = t.TempDir()
	})
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1600, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.launchLogBtnRect.Size[0] == 0 {
		t.Fatal("Launch log button not rendered in the detail panel")
	}

	clickRect(m.launchLogBtnRect, m.rootView)
	found := false
	for deadline := time.Now().Add(2 * time.Second); ; {
		for _, to := range sess.Snapshot().Toasts {
			if strings.Contains(to.Text, "no launch log") {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no-log toast never appeared: %+v", sess.Snapshot().Toasts)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestGUIDetailStopButton: while a supervised launch runs, the detail
// panel shows Stop; clicking it cancels the launch (the hook's context
// dies) and the button disappears (issue 068).
func TestGUIDetailStopButton(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{}, 1)
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.UmuLauncher = func(ctx context.Context, row ui.GameRow) (string, error) {
			started <- struct{}{}
			<-ctx.Done()
			return "", ctx.Err()
		}
	})
	// A manual game with a Windows exe is umu-eligible, so Launch
	// routes to the hook. Seeded via the games cache (the TUI tests'
	// pattern) — guiFakes only makes Steam rows, which never route.
	manualDir := filepath.Join(t.TempDir(), "WinGame")
	writeGUIFile(t, filepath.Join(manualDir, "game.exe"), "MZ")
	seedGUIGamesCache(t, root, []ui.GameRow{{
		Title: "WinGame", AppID: "manual_wingame",
		InstallDir: manualDir, Store: domain.StoreManual,
		ExePath: filepath.Join(manualDir, "game.exe"),
	}})
	sess.Start(context.Background())
	var row ui.GameRow
	for deadline := time.Now().Add(5 * time.Second); ; {
		for _, r := range sess.Snapshot().Rows {
			if r.InstallDir == manualDir {
				row = r
				break
			}
		}
		if row.InstallDir != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("manual game %s never appeared in rows: %+v", manualDir, sess.Snapshot().Rows)
		}
		time.Sleep(20 * time.Millisecond)
	}
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	sess.Launch(row.InstallDir)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("launch hook never ran")
	}
	if !sess.LaunchBusy(row.InstallDir) {
		t.Fatal("LaunchBusy = false while the game runs")
	}

	headlessFrames(t, 1600, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.stopBtnRect.Size[0] == 0 {
		t.Fatal("Stop button not rendered while the launch runs")
	}

	clickRect(m.stopBtnRect, m.rootView)
	for deadline := time.Now().Add(2 * time.Second); sess.LaunchBusy(row.InstallDir); {
		if time.Now().After(deadline) {
			t.Fatal("launch still busy after Stop")
		}
		time.Sleep(10 * time.Millisecond)
	}

	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.stopBtnRect.Size[0] != 0 {
		t.Error("Stop button still rendered after the launch stopped")
	}
}
