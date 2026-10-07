package ui

import (
	"context"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// Issue 028: manual per-game fixes for the rows identification and the
// cover chain cannot get right — a pinned display title (UI for
// settings.title_overrides) and user-uploaded poster art
// (settings.cover_overrides, which beats the whole fetch chain).

// SetTitleOverride pins title as the game's display title; an empty title
// clears the pin and the row title re-derives from the identification
// chain. The override persists, the row renames in place, and the cover
// re-resolves against the new title in the background.
func (s *Session) SetTitleOverride(gameDir, title string) {
	dir := canonicalDir(gameDir)
	row := s.findRow(dir)
	if row == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	title = strings.TrimSpace(title)

	s.mu.Lock()
	if title == "" {
		delete(s.deps.Settings.TitleOverrides, dir)
	} else {
		if s.deps.Settings.TitleOverrides == nil {
			s.deps.Settings.TitleOverrides = map[string]string{}
		}
		s.deps.Settings.TitleOverrides[dir] = title
	}
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}

	name, source := title, string(domain.SourceOverride)
	if name == "" {
		res := discovery.ChainResolver(nil)(dir, row.ExePath)
		name, source = res.Name, string(res.Source)
	}
	s.mu.Lock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Title = name
			s.st.Rows[i].TitleSource = source
			break
		}
	}
	sortRows(s.st.Rows)
	s.mu.Unlock()
	s.persistCache()
	if title == "" {
		s.toast("title override cleared: "+name, false)
	} else {
		s.toast("title set: "+title, false)
	}

	// The art bound to the old title may not match the new one (or a
	// placeholder may finally resolve): re-resolve in the background.
	go func() {
		row := s.findRow(dir)
		if row == nil {
			return
		}
		s.resolveCover(context.Background(), row)
		s.mu.Lock()
		for i := range s.st.Rows {
			if s.st.Rows[i].InstallDir == dir {
				s.st.Rows[i].CoverPath = row.CoverPath
				break
			}
		}
		s.mu.Unlock()
		s.persistCache()
		s.emit(Event{Kind: EvScanDone, Text: "title updated", GameDir: dir})
	}()
}

// SetCoverOverride pins a user-picked image as the game's poster. The
// image is validated and copied into the cover cache (normalized to the
// 2:3 invariant, the source untouched), the override persists, and the
// row rebinds immediately. The override beats the whole fetch chain and
// survives rescans; re-uploading replaces the same cache file.
func (s *Session) SetCoverOverride(gameDir, imagePath string) {
	dir := canonicalDir(gameDir)
	if s.findRow(dir) == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	if s.deps.Covers == nil {
		s.toast("poster: cover cache unavailable", true)
		return
	}
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return
	}
	name, err := s.deps.Covers.SetOverride(dir, imagePath)
	if err != nil {
		s.toast("poster: "+err.Error(), true)
		return
	}

	s.mu.Lock()
	if s.deps.Settings.CoverOverrides == nil {
		s.deps.Settings.CoverOverrides = map[string]string{}
	}
	s.deps.Settings.CoverOverrides[dir] = name
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}

	p, _ := s.deps.Covers.OverridePath(name)
	s.mu.Lock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].CoverPath = p
			break
		}
	}
	s.mu.Unlock()
	s.persistCache()
	s.toast("poster set: "+s.findRow(dir).Title, false)
	s.emit(Event{Kind: EvScanDone, Text: "poster updated", GameDir: dir})
}

// ClearCoverOverride removes the game's pinned poster: the settings entry
// and the cached copy go away, and the cover re-resolves through the
// fetch chain in the background.
func (s *Session) ClearCoverOverride(gameDir string) {
	dir := canonicalDir(gameDir)
	if s.findRow(dir) == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	s.mu.Lock()
	name := s.deps.Settings.CoverOverrides[dir]
	if name != "" {
		delete(s.deps.Settings.CoverOverrides, dir)
	}
	snap := s.deps.Settings
	s.mu.Unlock()
	if name == "" {
		s.toast("no custom poster to reset", false)
		return
	}
	if s.deps.Covers != nil {
		s.deps.Covers.ClearOverride(name)
	}
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}

	go func() {
		row := s.findRow(dir)
		if row == nil {
			return
		}
		s.resolveCover(context.Background(), row)
		s.mu.Lock()
		for i := range s.st.Rows {
			if s.st.Rows[i].InstallDir == dir {
				s.st.Rows[i].CoverPath = row.CoverPath
				break
			}
		}
		s.mu.Unlock()
		s.persistCache()
		s.emit(Event{Kind: EvScanDone, Text: "poster reset", GameDir: dir})
	}()
	s.toast("poster reset: "+s.findRow(dir).Title, false)
}

// CoverOverrideActive reports whether the game has a pinned poster (the
// "reset poster" affordance only makes sense then).
func (s *Session) CoverOverrideActive(gameDir string) bool {
	return s.Settings().CoverOverrides[canonicalDir(gameDir)] != ""
}

// PickAndSetCover opens the OS image picker and pins the choice as the
// game's poster, mirroring PickAndAddDirectory.
func (s *Session) PickAndSetCover(ctx context.Context, gameDir string) {
	go func() {
		path, err := s.pickFile(ctx)
		if err != nil {
			s.toast(err.Error(), true)
			return
		}
		if path == "" {
			return // cancelled
		}
		s.SetCoverOverride(gameDir, path)
	}()
}
