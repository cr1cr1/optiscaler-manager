package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// AddDirectory registers a user-picked directory and persists it in
// settings so later scans keep it. The directory is classified up front
// (stats and bounded walks only, no PE parsing — cheap enough for an
// explicit user action) and the kind decides the flow:
//
//   - game: the call never blocks on enrichment — validation, settings
//     persistence, and a placeholder row are synchronous (so a Scan started
//     right after sees the directory), while the walk/classify/cover
//     enrichment runs in a goroutine that replaces the placeholder and
//     settles with the usual EvScanDone "directory added" event;
//   - container: registered as a scan root (settings persisted
//     synchronously) with no placeholder or self-row, a "scan folder"
//     toast, and a background rescan that surfaces its games as rows;
//   - empty: refused with a warning toast; settings stay untouched.
//
// A classification failure falls through to the game flow, whose async
// error handling reports the problem. A duplicate Add of the same
// canonical dir while one is in flight is rejected with a toast and no
// event.
func (s *Session) AddDirectory(dir string) {
	root, err := canonicalDirChecked(dir)
	if err != nil {
		s.toast("add directory: "+err.Error(), true)
		return
	}
	kind, kerr := discovery.ClassifyGameDir(context.Background(), root)
	if kerr == nil {
		switch kind {
		case discovery.GameDirEmpty:
			s.toast("no games found under "+filepath.Base(root), true)
			return
		case discovery.GameDirContainer:
			s.addScanRoot(root)
			return
		}
	}
	ctx, ok := s.registerOp(root)
	if !ok {
		s.toast("add already in progress", true)
		return
	}
	s.mu.Lock()
	exists := false
	for _, r := range s.st.Rows {
		if r.InstallDir == root {
			exists = true
			break
		}
	}
	if exists {
		s.mu.Unlock()
		s.finishOp(root)
		s.toast(filepath.Base(root)+" already in library", false)
		return
	}
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	if !present {
		s.deps.Settings.ExtraDirs = append(s.deps.Settings.ExtraDirs, root)
	}
	snap := s.deps.Settings
	base := filepath.Base(root)
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:      base,
		AppID:      "custom_" + base,
		InstallDir: root,
		Platform:   domain.StoreManual.String(),
		Store:      domain.StoreManual,
	})
	sortRows(s.st.Rows)
	s.mu.Unlock()
	if !present {
		if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
			s.toast("settings not saved: "+err.Error(), true)
		}
	}
	go func() {
		snap := s.Settings()
		resolver := discovery.ChainResolver(func(d string) string {
			return snap.TitleOverrides[canonicalDir(d)]
		})
		entry, err := app.ManualEntryWithResolver(dir, s.deps.Store, resolver)
		if err != nil {
			s.finishOp(root)
			s.removeRow(root)
			s.toast("add directory: "+err.Error(), true)
			return
		}
		row := s.toRow(ctx, entry)
		if ctx.Err() != nil {
			s.finishOp(root)
			return // cancelled mid-add: the placeholder row stays for the next scan
		}
		s.mu.Lock()
		// A RemoveDirectory that landed while this add was enriching has
		// already dropped the placeholder row and the ExtraDirs entry;
		// re-appending now would resurrect a zombie row that survives until
		// the next scan. Skip the append, the cache write, and the event.
		registered := false
		for _, d := range s.deps.Settings.ExtraDirs {
			if d == root {
				registered = true
				break
			}
		}
		if !registered {
			s.mu.Unlock()
			s.finishOp(root)
			log.Debug().Str("dir", root).Msg("add settled after directory removal; row not appended")
			return
		}
		replaced := false
		for i := range s.st.Rows {
			if s.st.Rows[i].InstallDir == entry.Game.InstallDir {
				s.st.Rows[i] = row
				replaced = true
				break
			}
		}
		if !replaced {
			s.st.Rows = append(s.st.Rows, row)
		}
		sortRows(s.st.Rows)
		s.mu.Unlock()
		s.finishOp(root)
		s.persistCache()
		s.toast("added "+entry.Game.Name, false)
		s.emit(Event{Kind: EvScanDone, Text: "directory added"})
	}()
}

// addScanRoot registers a container directory as a recursive scan root:
// persisted in settings like a game add, but with no placeholder or
// self-row — the root's games surface as children of the background rescan
// it triggers (the rescan is the "directory added" equivalent: no
// EvScanDone text frontends could misread as a single-game add).
func (s *Session) addScanRoot(root string) {
	s.mu.Lock()
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	if !present {
		s.deps.Settings.ExtraDirs = append(s.deps.Settings.ExtraDirs, root)
	}
	snap := s.deps.Settings
	s.mu.Unlock()
	if !present {
		if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
			s.toast("settings not saved: "+err.Error(), true)
		}
	}
	s.toast("registered "+filepath.Base(root)+" as a scan folder", false)
	s.Scan(context.Background())
}

// removeRow drops the row for dir.
func (s *Session) removeRow(dir string) {
	s.mu.Lock()
	kept := make([]GameRow, 0, len(s.st.Rows))
	for _, r := range s.st.Rows {
		if r.InstallDir != dir {
			kept = append(kept, r)
		}
	}
	s.st.Rows = kept
	s.mu.Unlock()
}

// canonicalDirChecked canonicalizes p like canonicalDir and verifies it is
// an existing directory, mirroring the validation app.ManualEntry applies.
func canonicalDirChecked(p string) (string, error) {
	root := canonicalDir(p)
	st, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", root, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return root, nil
}

// RemoveDirectory unregisters a manually added directory: its row and any
// nested games scanned under it are dropped, settings persist without it,
// and the games cache is rewritten. Directories not in ExtraDirs are a
// silent no-op (no write, no event).
func (s *Session) RemoveDirectory(dir string) {
	root := canonicalDir(dir)
	s.mu.Lock()
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	s.mu.Unlock()
	if !present {
		return
	}
	// Abort any in-flight AddDirectory for this root before dropping the
	// row: its goroutine checks ctx.Err() before touching rows, so a cancel
	// here stops it from resurrecting the row after RemoveDirectory returns.
	s.CancelOp(root)
	s.mu.Lock()
	present = false
	kept := make([]string, 0, len(s.deps.Settings.ExtraDirs))
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			continue
		}
		kept = append(kept, d)
	}
	if !present {
		s.mu.Unlock()
		return // raced with a concurrent removal of the same root
	}
	s.deps.Settings.ExtraDirs = kept
	prefix := root + string(os.PathSeparator)
	rows := s.st.Rows[:0]
	for _, r := range s.st.Rows {
		if r.InstallDir == root || strings.HasPrefix(r.InstallDir, prefix) {
			continue
		}
		rows = append(rows, r)
	}
	s.st.Rows = rows
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	s.persistCache()
	s.toast("removed "+filepath.Base(root), false)
	s.emit(Event{Kind: EvScanDone, Text: "directory removed"})
}

// PickAndAddDirectory opens the OS directory picker and adds the choice.
func (s *Session) PickAndAddDirectory(ctx context.Context) {
	go func() {
		dir, err := s.pickDir(ctx)
		if err != nil {
			s.toast(err.Error(), true)
			return
		}
		if dir == "" {
			return // cancelled
		}
		s.AddDirectory(dir)
	}()
}
