package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dlssEnv wires a session whose DLSS client points at a fake NVIDIA/DLSS:
// /commits/main resolves to a fixed 40-hex sha and every raw file route
// serves a PE marked 310.9.1.0. The game's injection dir starts with the
// three runtime DLLs at 1.0.<i>.0. The Steam fixture (VDF + appmanifest,
// pinned SteamRoot) makes scans resolve the game locally, and offline
// settings keep scan enrichment off the network.
func dlssEnv(t *testing.T) (*ui.Session, string, string) {
	t.Helper()
	sha := strings.Repeat("a", 40)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":"%s/bundle","size":100}]}]`, "http://unused")
	})
	mux.HandleFunc("/repos/NVIDIA/DLSS/commits/main", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
	})
	mux.HandleFunc("/NVIDIA/DLSS/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	root := t.TempDir()
	gameRoot := filepath.Join(root, "steamapps", "common", "GameOne")
	bin := filepath.Join(gameRoot, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gameone.exe"), []byte("MZGAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"),
		[]byte(`"libraryfolders" { "0" { "path" "`+root+`" } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "steamapps", "appmanifest_100.acf"),
		[]byte(`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, name := range dlss.Files {
		if err := os.WriteFile(filepath.Join(bin, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prefs := settings.Defaults()
	prefs.OnlineLookups = false
	if err := settings.Save(filepath.Join(root, "settings"), prefs); err != nil {
		t.Fatal(err)
	}

	sess := ui.NewSession(ui.Deps{
		Store:        store.New(filepath.Join(root, "data")),
		GH:           gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), srv.URL),
		DLSS:         dlss.NewWithBaseURLs(nil, srv.URL, srv.URL),
		CacheDir:     filepath.Join(root, "cache"),
		SettingsRoot: filepath.Join(root, "settings"),
		SteamRoot:    root,
	})
	seedGamesCache(t, filepath.Join(root, "settings"), []ui.GameRow{{
		Title:        "Game One",
		InstallDir:   gameRoot,
		InjectionDir: bin,
		Status:       "committed",
		Components:   []string{"DLSS 1.0.0"},
		DLSSReady:    true, // mirrors the three runtime DLLs on disk
	}})
	sess.Start(context.Background())
	return sess, root, bin
}

// TestTUIDetailUpdateDLSS: 'u' on the detail screen updates the whole
// NVIDIA set at one commit and refreshes the row's DLSS component label.
func TestTUIDetailUpdateDLSS(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	m := Model{sess: sess, screen: screenDetail, detailDir: gameDirOf(sess)}
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	pollUntil(t, "updated NVIDIA set", func() bool {
		for _, name := range dlss.Files {
			v, err := fileVersion(filepath.Join(bin, name))
			if err != nil || v != "310.9.1.0" {
				return false
			}
		}
		return true
	})
	if snap := sess.Snapshot(); snap.Busy != "" {
		t.Errorf("busy %q after update settled", snap.Busy)
	}
	row := findRow(sess.Snapshot().Rows, gameDirOf(sess))
	// Rows carry the raw dll version in tag form (dlssLabel): 310.9.1.0
	// renders as "DLSS 310.9.1" — marketing names cannot reflect switches.
	if row == nil || !strings.Contains(strings.Join(row.Components, ","), "DLSS 310.9.1") {
		t.Errorf("components not refreshed: %v", row)
	}
}

// TestTUIDetailRestoreDLSSPick: 'p' on the detail screen opens the restore
// modal listing the game's DLSS backups, enter raises the session
// confirmation, and 'y' restores the complete backed-up set.
func TestTUIDetailRestoreDLSSPick(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	// Enter the detail screen through the real games-screen path so the
	// backup cache populates the way it does in production.
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Build a backup by running one update, then pump its settle event the
	// way the runtime event pump does so the cache refreshes.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})
	if len(m.backups) != 1 {
		t.Fatalf("cached %d backups after the update settle, want 1 (%v)", len(m.backups), m.backups)
	}
	if label := m.backups[0].Label; !strings.Contains(label, "DLSS 1.0.0") {
		t.Errorf("backup label %q does not name the backed-up version", label)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore == nil {
		t.Fatal("'p' opened no restore modal")
	}
	out := m.View()
	if !strings.Contains(out, "Restore NVIDIA DLSS backup") || !strings.Contains(out, m.backups[0].Label) {
		t.Errorf("restore modal lacks the title or the backup label:\n%s", out)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if sess.Snapshot().Confirm == nil {
		t.Fatal("restore did not raise the session confirmation")
	}
	// Consent has not been given yet: no op may run, no byte may move.
	if snap := sess.Snapshot(); snap.Busy != "" {
		t.Errorf("restore dispatch started an op before consent (busy %q)", snap.Busy)
	}
	if v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll")); err != nil || v != "310.9.1.0" {
		t.Errorf("files changed before consent: %q (%v)", v, err)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	pollUntil(t, "restored NVIDIA set", func() bool {
		for i, name := range dlss.Files {
			v, err := fileVersion(filepath.Join(bin, name))
			want := "1.0." + string(rune('0'+i)) + ".0"
			if err != nil || v != want {
				return false
			}
		}
		return true
	})
}

// TestTUIRestorePickNavigation: the newest backup is highlighted first,
// up/down move the highlight, and enter dispatches the highlighted
// snapshot id into the session confirmation.
func TestTUIRestorePickNavigation(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Two backups: the update creates the first (the 1.0.x set); restoring
	// it afterwards backs up the updated set before the restore lands.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	pollUntil(t, "restore settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "1.0.0.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})
	if len(m.backups) != 2 {
		t.Fatalf("cached %d backups after the restore settle, want 2 (%v)", len(m.backups), m.backups)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore == nil || m.restore.sel != 0 {
		t.Fatalf("'p' opened the modal at %+v, want sel 0", m.restore)
	}
	second := m.restore.items[1]
	if !strings.Contains(second.Label, "DLSS 1.0.0") {
		t.Fatalf("second entry %q is not the original set", second.Label)
	}
	out := m.View()
	if !strings.Contains(out, second.Label) {
		t.Errorf("restore modal does not list the second entry:\n%s", out)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.restore.sel != 1 {
		t.Fatalf("'j' left sel at %d, want 1", m.restore.sel)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.restore.sel != 1 {
		t.Fatalf("'j' at the last entry wrapped to %d, want 1", m.restore.sel)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.restore.sel != 0 {
		t.Fatalf("'k' left sel at %d, want 0", m.restore.sel)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.restore.sel != 1 {
		t.Fatalf("'j' after 'k' left sel at %d, want 1", m.restore.sel)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	confirm := sess.Snapshot().Confirm
	if confirm == nil {
		t.Fatal("enter raised no confirmation")
	}
	if confirm.SnapshotID != second.ID {
		t.Errorf("confirm targets %q, want the highlighted %q", confirm.SnapshotID, second.ID)
	}
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // decline, nothing more to prove
}

// TestTUIRestorePickModalKeys: while the modal is open every other key is
// swallowed, and esc closes it without raising a confirmation or an op.
func TestTUIRestorePickModalKeys(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore == nil {
		t.Fatal("'p' opened no restore modal")
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = nm.(Model)
	if cmd != nil {
		t.Error("'q' while the modal is open started quitting")
	}
	if m.restore == nil || m.screen != screenDetail {
		t.Error("'q' while the modal is open was not swallowed")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.cycle != nil {
		t.Error("'v' while the modal is open staged a version cycle")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.restore != nil {
		t.Fatal("esc did not close the restore modal")
	}
	if snap := sess.Snapshot(); snap.Confirm != nil || snap.Busy != "" {
		t.Errorf("esc raised a confirmation or an op (confirm %+v, busy %q)", snap.Confirm, snap.Busy)
	}
}

// TestTUIDetailRestoreDimmedWithoutBackups: a DLSS-ready game with no
// backups renders the restore action dimmed and 'p' does nothing.
func TestTUIDetailRestoreDimmedWithoutBackups(t *testing.T) {
	sess, _, _ := dlssEnv(t)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if out := m.View(); !strings.Contains(out, "(no backups)") {
		t.Errorf("detail view does not mark restore as backup-less:\n%s", out)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore != nil {
		t.Error("'p' opened a restore modal with no backups")
	}
}

// TestTUIRestorePickGatedOnDLSSReady: 'p' opens the picker only while the
// row is DLSS-ready — the same condition the detail menu's enabled state
// renders — even when the update left a real backup behind.
func TestTUIRestorePickGatedOnDLSSReady(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})
	if len(m.backups) != 1 {
		t.Fatalf("cached %d backups after the update settle, want 1 (%v)", len(m.backups), m.backups)
	}

	// The set loses a DLL by hand and a rescan re-probes readiness: the
	// row is no longer DLSS-ready, while the update's backup still exists.
	if err := os.Remove(filepath.Join(bin, "nvngx_dlssg.dll")); err != nil {
		t.Fatal(err)
	}
	sess.Scan(context.Background())
	pollUntil(t, "rescan marks the row not DLSS-ready", func() bool {
		row := findRow(sess.Snapshot().Rows, dir)
		return row != nil && !row.DLSSReady
	})

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore != nil {
		t.Error("'p' opened a restore modal for a not-DLSS-ready row")
	}
}

// TestTUIRestorePickPopulatesOnDetailEntry: a game whose backups predate
// the visit lists them in the picker from the cache filled on detail
// entry — no settle event needed during this visit.
func TestTUIRestorePickPopulatesOnDetailEntry(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	// First visit: run the update that leaves a backup behind, and pump
	// its settle so the exit cache is warm.
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})

	// Second visit: entry alone fills the cache; the picker lists the
	// backup with no settle event pumped in this visit.
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore == nil || len(m.restore.items) != 1 {
		t.Fatalf("'p' after re-entry: %+v, want the one cached backup", m.restore)
	}
	if out := m.View(); !strings.Contains(out, m.restore.items[0].Label) {
		t.Errorf("restore modal does not list the cached backup:\n%s", out)
	}
}

// TestTUIRestorePickResyncsOnSettle: a picker opened while an op is still
// in flight shows the pre-op cache; the settle event re-syncs the open
// modal with the fresh list (updates and restores both add a backup).
func TestTUIRestorePickResyncsOnSettle(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})

	// Dispatch a restore of the original set, then re-open the picker
	// before the settle event arrives: the modal still shows the pre-op
	// cache (one entry).
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if sess.Snapshot().Confirm == nil {
		t.Fatal("restore raised no confirmation")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	pollUntil(t, "restore settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "1.0.0.0"
	})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.restore == nil || len(m.restore.items) != 1 {
		t.Fatalf("picker before settle: %+v, want the single pre-op entry", m.restore)
	}

	// The settle event refreshes the cache and the open modal with it.
	m = pumpEvent(m, ui.Event{Kind: ui.EvOpDone, GameDir: dir})
	if m.restore == nil {
		t.Fatal("settle closed the open restore modal")
	}
	if len(m.restore.items) != 2 {
		t.Fatalf("open picker shows %d entries after the settle, want 2 (%v)", len(m.restore.items), m.restore.items)
	}
	if m.restore.sel >= len(m.restore.items) {
		t.Errorf("sel %d out of range after re-sync", m.restore.sel)
	}
}

// pumpEvent feeds one session event through Update the way the runtime
// event pump (waitEvent) does.
func pumpEvent(m Model, ev ui.Event) Model {
	nm, _ := m.Update(eventMsg(ev))
	return nm.(Model)
}

// TestTUIGamesKeyUpdateDLSS: 'u' on the games screen mirrors the card action.
func TestTUIGamesKeyUpdateDLSS(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	m := Model{sess: sess, screen: screenGames, cursor: 0}
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "updated NVIDIA set", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})
}

// TestTUIDetailUpdateDLSSInstallsOverMissing: a partial current set does
// not block the update — it warns (log), installs the missing members with
// the rest, and writes no backup of the incomplete set.
func TestTUIDetailUpdateDLSSInstallsOverMissing(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	if err := os.Remove(filepath.Join(bin, "nvngx_dlssg.dll")); err != nil {
		t.Fatal(err)
	}
	m := Model{sess: sess, screen: screenDetail, detailDir: gameDirOf(sess)}
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "updated NVIDIA set", func() bool {
		for _, name := range dlss.Files {
			v, err := fileVersion(filepath.Join(bin, name))
			if err != nil || v != "310.9.1.0" {
				return false
			}
		}
		return true
	})
	if _, err := os.Stat(filepath.Join(bin, "dlss-backups")); !os.IsNotExist(err) {
		t.Errorf("backup-less update must not create backups (stat err %v)", err)
	}
}

// gameDirOf returns the seeded row's install dir (the detail target).
func gameDirOf(sess *ui.Session) string {
	return sess.Snapshot().Rows[0].InstallDir
}

// fileVersion wraps pever.FileVersion so poll predicates read as false, not fatal.
func fileVersion(path string) (string, error) {
	return pever.FileVersion(path)
}

// TestRestoreBoxWindowsLongLists: the modal caps the rendered list around
// the selection so a long backup history cannot overflow the terminal.
func TestRestoreBoxWindowsLongLists(t *testing.T) {
	items := make([]stagedItem, 10)
	for i := range items {
		items[i] = stagedItem{ID: fmt.Sprintf("snap-%d", i), Label: fmt.Sprintf("backup-%d", i)}
	}
	out := restoreBox(&restorePick{items: items, sel: 0})
	if strings.Contains(out, "backup-9") {
		t.Error("restore box rendered the whole list with sel at the top (overflow)")
	}
	if !strings.Contains(out, "backup-0") {
		t.Error("restore box lost the selected entry")
	}
	out = restoreBox(&restorePick{items: items, sel: 9})
	if strings.Contains(out, "backup-0") {
		t.Error("restore box rendered the whole list with sel at the bottom (overflow)")
	}
	if !strings.Contains(out, "backup-9") {
		t.Error("restore box lost the selected entry")
	}
}
