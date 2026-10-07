package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// newOverrideEnv wires a session with the cover fake (art only for
// "Cyberpunk 2077"/1091500) plus real settings/cache roots.
func newOverrideEnv(t *testing.T) (*Session, *coverFake) {
	t.Helper()
	f := newCoverFake(t)
	root := t.TempDir()
	s := NewSession(Deps{
		Store:        store.New(root),
		Covers:       f.covers(t),
		CacheDir:     filepath.Join(root, "cache"),
		SettingsRoot: filepath.Join(root, "settings"),
	})
	return s, f
}

// addManualGame adds a manual game dir whose folder name is its title
// (generic "game.exe" stem keeps the folder as the resolved title) and
// waits for the async enrichment to settle its cover.
func addManualGame(t *testing.T, s *Session, folder string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), folder)
	writeUIFile(t, filepath.Join(dir, "game.exe"), "GAME")
	s.AddDirectory(dir)
	root := canonicalDir(dir)
	pollUITill(t, "enrichment to settle the cover", func() bool {
		row := s.findRow(root)
		return row != nil && row.CoverPath != ""
	})
	return root
}

func pollUITill(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// writePNG encodes a small decodable portrait image.
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Issue 028: SetTitleOverride renames the row in place, persists the
// override, and re-resolves the cover with the new title.
func TestSetTitleOverrideRenamesPersistsRefetches(t *testing.T) {
	s, f := newOverrideEnv(t)
	root := addManualGame(t, s, "Whatever") // no chain art for this title

	s.SetTitleOverride(root, "Cyberpunk 2077")

	row := s.findRow(root)
	if row == nil || row.Title != "Cyberpunk 2077" {
		t.Fatalf("row title = %+v, want Cyberpunk 2077", row)
	}
	if row.TitleSource != "override" {
		t.Errorf("TitleSource = %q, want override", row.TitleSource)
	}
	loaded, err := settings.Load(s.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TitleOverrides[root] != "Cyberpunk 2077" {
		t.Errorf("persisted TitleOverrides = %v", loaded.TitleOverrides)
	}
	pollUITill(t, "cover re-resolution with the new title", func() bool {
		row := s.findRow(root)
		return row != nil && strings.Contains(row.CoverPath, f.knownAppID)
	})
}

// Issue 028: an empty title clears the override; the row title falls back
// to the identification chain.
func TestSetTitleOverrideEmptyClears(t *testing.T) {
	s, _ := newOverrideEnv(t)
	root := addManualGame(t, s, "Whatever")

	s.SetTitleOverride(root, "Pinned")
	s.SetTitleOverride(root, "")

	loaded, err := settings.Load(s.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.TitleOverrides[root]; ok {
		t.Errorf("override still persisted after clear: %v", loaded.TitleOverrides)
	}
	row := s.findRow(root)
	if row == nil || row.Title != "Whatever" {
		t.Errorf("row title after clear = %+v, want chain-derived Whatever", row)
	}
}

// Issue 028: a user-uploaded poster is copied into the cache, persisted,
// and beats the fetch chain — across explicit cover re-resolutions.
func TestSetCoverOverrideStickyAcrossRescan(t *testing.T) {
	s, f := newOverrideEnv(t)
	root := addManualGame(t, s, "Cyberpunk 2077") // chain art EXISTS for this one
	if row := s.findRow(root); row == nil || !strings.Contains(row.CoverPath, f.knownAppID) {
		t.Fatalf("precondition: chain art expected, row %+v", row)
	}

	src := filepath.Join(t.TempDir(), "poster.png")
	writePNG(t, src, 60, 90)
	s.SetCoverOverride(root, src)

	row := s.findRow(root)
	if row == nil || !strings.Contains(filepath.Base(row.CoverPath), "override_") {
		t.Fatalf("row cover = %+v, want the uploaded override", row)
	}
	loaded, err := settings.Load(s.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CoverOverrides[root] == "" {
		t.Errorf("cover_overrides not persisted: %v", loaded.CoverOverrides)
	}
	// A rescan-style cover re-resolution must not clobber user art.
	again := s.findRow(root)
	s.resolveCover(context.Background(), again)
	if !strings.Contains(filepath.Base(again.CoverPath), "override_") {
		t.Errorf("cover after re-resolution = %q, want the sticky override", again.CoverPath)
	}
}

// Issue 028: clearing the override removes the cached copy and the fetch
// chain takes over again.
func TestClearCoverOverrideRestoresChain(t *testing.T) {
	s, f := newOverrideEnv(t)
	root := addManualGame(t, s, "Cyberpunk 2077")
	src := filepath.Join(t.TempDir(), "poster.png")
	writePNG(t, src, 60, 90)
	s.SetCoverOverride(root, src)
	loaded, _ := settings.Load(s.deps.SettingsRoot)
	name := loaded.CoverOverrides[root]
	if name == "" {
		t.Fatal("precondition: override persisted")
	}

	s.ClearCoverOverride(root)

	loaded, _ = settings.Load(s.deps.SettingsRoot)
	if _, ok := loaded.CoverOverrides[root]; ok {
		t.Errorf("override still persisted after clear: %v", loaded.CoverOverrides)
	}
	row := s.findRow(root)
	s.resolveCover(context.Background(), row)
	if !strings.Contains(row.CoverPath, f.knownAppID) {
		t.Errorf("cover after clear = %q, want chain art %s", row.CoverPath, f.knownAppID)
	}
}

// Issue 028: a non-image upload is rejected — row and settings untouched.
func TestSetCoverOverrideRejectsNonImage(t *testing.T) {
	s, _ := newOverrideEnv(t)
	root := addManualGame(t, s, "Whatever")
	before := s.findRow(root).CoverPath

	junk := filepath.Join(t.TempDir(), "poster.png")
	if err := os.WriteFile(junk, []byte("definitely not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.SetCoverOverride(root, junk)

	if row := s.findRow(root); row.CoverPath != before {
		t.Errorf("cover changed on rejected upload: %q → %q", before, row.CoverPath)
	}
	loaded, _ := settings.Load(s.deps.SettingsRoot)
	if len(loaded.CoverOverrides) != 0 {
		t.Errorf("rejected upload persisted: %v", loaded.CoverOverrides)
	}
}
