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
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dlssEnv wires a session whose DLSS client points at a fake NVIDIA/DLSS:
// /commits/main resolves to a fixed 40-hex sha and every raw file route
// serves a PE marked 310.9.1.0. The game's injection dir starts with the
// three runtime DLLs at 1.0.<i>.0.
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
	for i, name := range dlss.Files {
		if err := os.WriteFile(filepath.Join(bin, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sess := ui.NewSession(ui.Deps{
		Store:        store.New(filepath.Join(root, "data")),
		GH:           gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), srv.URL),
		DLSS:         dlss.NewWithBaseURLs(nil, srv.URL, srv.URL),
		CacheDir:     filepath.Join(root, "cache"),
		SettingsRoot: filepath.Join(root, "settings"),
	})
	seedGamesCache(t, filepath.Join(root, "settings"), []ui.GameRow{{
		Title:        "Game One",
		InstallDir:   gameRoot,
		InjectionDir: bin,
		Status:       "committed",
		Components:   []string{"DLSS 1.0.0"},
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
	// Rows carry the vendor marketing name (pever.MarketingName); 310.9.1
	// is past the vendored table, so the nearest-below tier renders DLSS 4.5.
	if row == nil || !strings.Contains(strings.Join(row.Components, ","), "DLSS 4.5") {
		t.Errorf("components not refreshed: %v", row)
	}
}

// TestTUIDetailRestoreDLSSPick: 'p' stages a restore pick from the session's
// snapshot list, the staged line renders, Enter raises the session
// confirmation, and 'y' restores the complete backed-up set.
func TestTUIDetailRestoreDLSSPick(t *testing.T) {
	sess, _, bin := dlssEnv(t)
	dir := gameDirOf(sess)
	m := Model{sess: sess, screen: screenDetail, detailDir: dir}

	// Build a backup to restore by running one update first.
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "update settled", func() bool {
		v, err := fileVersion(filepath.Join(bin, "nvngx_dlss.dll"))
		return err == nil && v == "310.9.1.0"
	})

	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.cycle == nil || !m.cycle.restore {
		t.Fatalf("'p' staged no restore pick (cycle %+v)", m.cycle)
	}
	if label := m.cycle.items[m.cycle.idx].Label; !strings.Contains(label, "DLSS 1.0.0") {
		t.Errorf("staged restore label %q does not name the backed-up version", label)
	}
	out := m.detailView(80, 24)
	if !strings.Contains(out, "restore DLSS 1.0.0") {
		t.Errorf("detail view lacks the staged restore line: %s", out)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if sess.Snapshot().Confirm == nil {
		t.Fatal("restore did not raise the session confirmation")
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

// TestTUIDetailUpdateDLSSRefusesMissing: without the complete set on disk
// the update refuses with a warning toast and writes nothing.
func TestTUIDetailUpdateDLSSRefusesMissing(t *testing.T) {
	sess, root, bin := dlssEnv(t)
	if err := os.Remove(filepath.Join(bin, "nvngx_dlssg.dll")); err != nil {
		t.Fatal(err)
	}
	m := Model{sess: sess, screen: screenDetail, detailDir: gameDirOf(sess)}
	_ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	pollUntil(t, "warning toast", func() bool {
		snap := sess.Snapshot()
		for _, to := range snap.Toasts {
			if to.Warn && strings.Contains(to.Text, "nvngx_dlssg.dll is missing") {
				return true
			}
		}
		return false
	})
	if _, err := os.Stat(filepath.Join(root, "settings", "dlss-backups")); !os.IsNotExist(err) {
		t.Errorf("refused update must not create backups (stat err %v)", err)
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
