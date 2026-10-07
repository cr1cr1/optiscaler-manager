package tui

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// writePosterPNG encodes a small decodable portrait image for the
// poster-upload tests.
func writePosterPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 60, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// Issue 028: 't' on the detail screen opens a title input pre-filled with
// the row title; committing renames the row and persists the override.
func TestDetailKeySetTitle(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title: "ControlLauncher", AppID: "manual_control",
		InstallDir: e.gameRoot, Platform: "manual",
	}})
	e.sess.Start(context.Background())
	pollUntil(t, "cached row to load", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})

	m := New(e.sess, "v0.0.0-test")
	m.screen = screenDetail
	m.detailDir = e.gameRoot
	mi, _ := m.detailKey(runeKey('t'))
	m = mi.(Model)
	if m.mode != inputSetTitle {
		t.Fatalf("mode = %v after 't', want inputSetTitle", m.mode)
	}
	if m.input.Value() != "ControlLauncher" {
		t.Errorf("input pre-fill = %q, want the row title", m.input.Value())
	}
	m.input.SetValue("Control")
	m.commitInput()

	pollUntil(t, "renamed row", func() bool {
		row := findRow(e.sess.Snapshot().Rows, e.gameRoot)
		return row != nil && row.Title == "Control"
	})
	loaded, err := settings.Load(settingsDir)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, v := range loaded.TitleOverrides {
		got = v
	}
	if got != "Control" {
		t.Errorf("persisted TitleOverrides = %v", loaded.TitleOverrides)
	}
}

// Issue 028: committing an empty title clears the override.
func TestDetailKeySetTitleEmptyClears(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title: "Game One", AppID: "100", InstallDir: e.gameRoot, Platform: "Steam",
	}})
	e.sess.Start(context.Background())
	pollUntil(t, "cached row to load", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})

	m := New(e.sess, "v0.0.0-test")
	m.screen = screenDetail
	m.detailDir = e.gameRoot
	mi, _ := m.detailKey(runeKey('t'))
	m = mi.(Model)
	m.input.SetValue("")
	m.commitInput()

	loaded, err := settings.Load(settingsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.TitleOverrides) != 0 {
		t.Errorf("TitleOverrides = %v after empty commit, want none", loaded.TitleOverrides)
	}
}

// Issue 028: 'a' on the detail screen opens a poster-path input; a valid
// image becomes the row's cover immediately.
func TestDetailKeySetPoster(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title: "Game One", AppID: "100", InstallDir: e.gameRoot, Platform: "Steam",
	}})
	e.sess.Start(context.Background())
	pollUntil(t, "cached row to load", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})

	src := filepath.Join(t.TempDir(), "poster.png")
	writePosterPNG(t, src)

	m := New(e.sess, "v0.0.0-test")
	m.screen = screenDetail
	m.detailDir = e.gameRoot
	mi, _ := m.detailKey(runeKey('a'))
	m = mi.(Model)
	if m.mode != inputSetCover {
		t.Fatalf("mode = %v after 'a', want inputSetCover", m.mode)
	}
	m.input.SetValue(src)
	m.commitInput()

	pollUntil(t, "override cover", func() bool {
		row := findRow(e.sess.Snapshot().Rows, e.gameRoot)
		return row != nil && strings.Contains(filepath.Base(row.CoverPath), "override_")
	})
}

// Issue 028: the detail view's action list documents the new bindings.
func TestDetailViewListsTitlePosterActions(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title: "Game One", AppID: "100", InstallDir: e.gameRoot, Platform: "Steam",
	}})
	e.sess.Start(context.Background())
	pollUntil(t, "cached row to load", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})

	m := New(e.sess, "v0.0.0-test")
	m.screen = screenDetail
	m.detailDir = e.gameRoot
	view := m.detailView(100, 40)
	if !strings.Contains(view, "set title") {
		t.Errorf("detail view lacks the set-title action:\n%s", view)
	}
	if !strings.Contains(view, "set poster") {
		t.Errorf("detail view lacks the set-poster action:\n%s", view)
	}
}
