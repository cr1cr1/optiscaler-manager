package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// Start boots the library: a warm games cache hydrates the rows
// synchronously (status reconciled from store manifests — no PE parsing, no
// reclassification) and no scan runs; a missing or unusable cache falls
// through to Scan. Safe to call once at frontend boot.
func (s *Session) Start(ctx context.Context) {
	rows := loadGamesCache(s.deps.SettingsRoot, s.deps.GOOS)
	if len(rows) == 0 {
		s.Scan(ctx)
		return
	}
	s.reconcileStatuses(rows)
	sortRows(rows)
	s.mu.Lock()
	s.st.Rows = rows
	s.st.StatusLine = fmt.Sprintf("%d games (cached)", len(rows))
	s.st.Busy = ""
	s.mu.Unlock()
	s.toastInterrupted(rows)
	// Resolve the default version so the memo is populated for the version
	// dropdown before the next manual scan. Async: Start must not block on
	// the resolve network call.
	go s.warmBootResolveDefault(ctx)
}

// toastInterrupted surfaces installs left in in_progress/failed state at
// boot: the process died mid-transaction, and only the user can choose
// repair/rollback/retry (docs/safety.md), so the boot warns and the
// actionable rows carry the Rollback affordance. Nothing is auto-deleted.
func (s *Session) toastInterrupted(rows []GameRow) {
	if msg := InterruptedMessage(InterruptedRows(rows)); msg != "" {
		s.toast(msg, true)
	}
}

// reconcileStatuses overrides cached row status from store manifests keyed
// by canonical install dir (and game root), so installs that settled while
// the manager was not running show their real state.
func (s *Session) reconcileStatuses(rows []GameRow) {
	if s.deps.Store == nil {
		return
	}
	manifests, err := s.deps.Store.List()
	if err != nil {
		log.Warn().Err(err).Msg("games cache: status reconcile skipped")
		return
	}
	byDir := map[string]domain.Status{}
	byRoot := map[string]domain.Status{}
	for _, m := range manifests {
		byDir[m.InstallDir] = m.Status
		byRoot[m.GameRoot] = m.Status
	}
	for i := range rows {
		st, ok := byDir[rows[i].InjectionDir]
		if !ok {
			st, ok = byRoot[rows[i].InstallDir]
		}
		if ok {
			rows[i].Status = st
			rows[i].Actionable = actionableStatus(st)
		}
		// The disabled flag lives on disk (a renamed hook), not in the
		// manifest: re-probe installed rows so a toggle done while the
		// manager was not running renders correctly.
		if rows[i].InjectionDir != "" && (rows[i].Status == domain.StatusCommitted || rows[i].Status == domain.StatusExternal) {
			_, f := pever.DisabledHook(rows[i].InjectionDir)
			rows[i].Disabled = f != ""
		}
	}
}

// Scan refreshes the library asynchronously. Scans are serialized: a Scan
// landing while one is in flight sets a pending bit instead of spawning a
// second goroutine, and the running scan re-runs once when it settles
// (success or failure). A container added mid-scan is therefore surfaced by
// a scan whose settings snapshot already includes it — an earlier scan
// settling last can no longer wipe freshly surfaced rows — and concurrent
// scans never thrash the busy/progress state. Only scans that actually run
// emit EvScanStarted/EvScanDone; a coalesced call emits nothing.
func (s *Session) Scan(ctx context.Context) {
	s.scanMu.Lock()
	if s.scanning {
		s.scanPending = true
		s.scanMu.Unlock()
		return
	}
	s.scanning = true
	s.scanMu.Unlock()
	go func() {
		for {
			s.runScan(ctx)
			s.scanMu.Lock()
			if !s.scanPending {
				s.scanning = false
				s.scanMu.Unlock()
				return
			}
			s.scanPending = false
			s.scanMu.Unlock()
			// The pending re-run's trigger may outlive the caller's ctx.
			ctx = context.Background()
		}
	}()
}

// scanIdle reports whether no scan is running and none is pending.
func (s *Session) scanIdle() bool {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	return !s.scanning && !s.scanPending
}

// runScan performs one scan pass, emitting EvScanStarted up front and
// EvScanDone/EvScanFailed on settle.
func (s *Session) runScan(ctx context.Context) {
	s.emit(Event{Kind: EvScanStarted})
	s.setBusy("Scanning…")
	s.resetProgress()
	snap := s.Settings()
	// Resolve the default OptiScaler version once per scan (never per row
	// or frame); toRow compares every installed row against the memo.
	// Gated on online lookups like the rest of the scan's network work.
	if snap.OnlineLookups {
		s.refreshResolvedDefault(ctx, snap.DefaultVersion)
	} else {
		s.memoizePinnedDefault(snap.DefaultVersion)
	}
	resolver := discovery.ChainResolver(func(dir string) string {
		return snap.TitleOverrides[canonicalDir(dir)]
	})
	entries, err := app.ScanAllLibraries(ctx, s.deps.Store, app.ScanAllOptions{
		SteamRoot: s.deps.SteamRoot,
		ExtraDirs: snap.ExtraDirs,
		Progress: func(phase string, done, total int) {
			if ctx.Err() != nil {
				return
			}
			s.scanProgress(phase, done, total)
		},
		Resolver: resolver,
	})
	if err != nil {
		if errors.Is(err, app.ErrNoGames) {
			entries = nil // empty first-run library: settle at 0 games
		} else {
			s.clearProgress()
			s.setBusy("")
			s.setStatus("Scan failed: " + err.Error())
			log.Warn().Err(err).Msg("scan failed")
			s.emit(Event{Kind: EvScanFailed, Text: err.Error()})
			return
		}
	}
	// Classify each extra root once (stats and bounded walks only, no PE
	// parsing): container/empty roots are scan roots whose games already
	// surfaced via the recursive scan — they get no self-row from
	// mergeExtraDirs, no cover tick, and stale self-rows are not
	// resurrected by the in-flight keep below. Roots that fail
	// classification keep the previous row-bearing behavior.
	scanOnlyRoots := map[string]bool{}
	for _, d := range snap.ExtraDirs {
		kind, err := discovery.ClassifyGameDir(ctx, d)
		if err == nil && kind != discovery.GameDirGame {
			scanOnlyRoots[d] = true
		}
	}
	coversTotal := len(entries) + len(snap.ExtraDirs) - len(scanOnlyRoots)
	coversDone := 0
	coversTick := func() {
		coversDone++
		s.scanProgress(phaseCovers, coversDone, coversTotal)
	}
	rows := make([]GameRow, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			s.clearProgress()
			s.setBusy("")
			s.setStatus("Scan failed: " + err.Error())
			log.Warn().Err(err).Msg("scan cancelled")
			s.emit(Event{Kind: EvScanFailed, Text: err.Error()})
			return
		}
		rows = append(rows, s.toRow(ctx, e))
		coversTick()
	}
	rows = s.mergeExtraDirs(ctx, rows, snap.ExtraDirs, scanOnlyRoots, coversTick, resolver)
	// Online lookup phase: enrich the local rows before they are
	// committed to state, so the final persistCache lands the enriched
	// fields in one write.
	s.enrichOnline(ctx, rows, snap)
	s.mu.Lock()
	// Directories added while this scan was in flight are not in the
	// snapshot's ExtraDirs; keep their rows rather than wiping them.
	// Rows for roots the snapshot classified as container/empty are
	// stale self-rows from before the gate existed — drop them.
	fresh := map[string]bool{}
	for _, r := range rows {
		fresh[r.InstallDir] = true
	}
	for _, r := range s.st.Rows {
		if fresh[r.InstallDir] {
			continue
		}
		for _, d := range s.deps.Settings.ExtraDirs {
			if r.InstallDir == d && !scanOnlyRoots[d] {
				rows = append(rows, r)
				break
			}
		}
	}
	sortRows(rows)
	disambiguateTitles(rows)
	s.refreshCovers(ctx, rows)
	s.st.Rows = rows
	s.st.StatusLine = fmt.Sprintf("%d games", len(rows))
	s.st.Busy = ""
	s.st.Progress = nil
	s.mu.Unlock()
	s.persistCache()
	s.emit(Event{Kind: EvScanDone, Text: fmt.Sprintf("%d games", len(rows))})
}

// scanProgress records one pipeline tick. State.Progress is authoritative
// and updated on every tick; an EvScanProgress poke is emitted at most
// every progressPokeInterval or on a phase change.
func (s *Session) scanProgress(phase string, done, total int) {
	s.mu.Lock()
	changed := s.st.Progress == nil || s.st.Progress.Phase != phase
	s.st.Progress = &ScanProgress{Phase: phase, Done: done, Total: total}
	s.mu.Unlock()
	s.progressMu.Lock()
	due := changed || time.Since(s.lastPoke) >= progressPokeInterval
	if due {
		s.lastPoke = time.Now()
	}
	s.progressMu.Unlock()
	if due {
		s.emit(Event{Kind: EvScanProgress})
	}
}

// resetProgress arms the poke throttle for a new scan.
func (s *Session) resetProgress() {
	s.progressMu.Lock()
	s.lastPoke = time.Time{}
	s.progressMu.Unlock()
	s.mu.Lock()
	s.st.Progress = nil
	s.mu.Unlock()
}

// clearProgress drops the progress snapshot; callers invoke it before the
// terminal scan event so frontends observe nil on completion.
func (s *Session) clearProgress() {
	s.mu.Lock()
	s.st.Progress = nil
	s.mu.Unlock()
}

// mergeExtraDirs appends rows for game directories the user added manually
// and the scan did not surface. Roots in scanOnly (classified container or
// empty by the caller) are scan roots, not games: they get no self-row.
// extraDirs must be a locked settings snapshot taken by the caller. tick,
// when non-nil, runs once per non-scanOnly root (row appended, deduplicated,
// or failed — the caller's total counts all of them).
func (s *Session) mergeExtraDirs(ctx context.Context, rows []GameRow, extraDirs []string, scanOnly map[string]bool, tick func(), res discovery.TitleResolver) []GameRow {
	for _, d := range extraDirs {
		if scanOnly[d] {
			continue
		}
		entry, err := app.ManualEntryWithResolver(d, s.deps.Store, res)
		if err != nil {
			log.Warn().Err(err).Str("dir", d).Msg("extra dir unavailable")
		} else {
			dup := false
			for _, r := range rows {
				if r.InstallDir == entry.Game.InstallDir {
					dup = true
					break
				}
			}
			if !dup {
				rows = append(rows, s.toRow(ctx, entry))
			}
		}
		if tick != nil {
			tick()
		}
	}
	return rows
}

// disambiguateTitles appends a folder suffix to rows that share one
// title, so games whose exes carry identical metadata titles (both
// "TOI") stay distinguishable in the library. When the folder name is
// the title itself, the parent directory disambiguates ("Red Dead
// Redemption 2 (Games)" vs "(common)"); the full install dir is the
// last resort.
func disambiguateTitles(rows []GameRow) {
	squeeze := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' {
				return -1
			}
			return unicode.ToLower(r)
		}, s)
	}
	groups := map[string][]int{}
	for i, r := range rows {
		groups[r.Title] = append(groups[r.Title], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		seen := map[string]bool{}
		for _, i := range idxs {
			suffix := filepath.Base(rows[i].InstallDir)
			if squeeze(suffix) == squeeze(rows[i].Title) {
				if parent := filepath.Base(filepath.Dir(rows[i].InstallDir)); parent != "" && squeeze(parent) != squeeze(rows[i].Title) {
					suffix = parent
				} else {
					suffix = rows[i].InstallDir
				}
			}
			for seen[suffix] {
				suffix = rows[i].InstallDir
			}
			seen[suffix] = true
			rows[i].Title += " (" + suffix + ")"
		}
	}
}

// toRow enriches a library entry into display form, resolving cover art.
func (s *Session) toRow(ctx context.Context, e app.LibraryEntry) GameRow {
	row := GameRow{
		Title:             e.Game.Name,
		AppID:             e.Game.AppID,
		InstallDir:        e.Game.InstallDir,
		InjectionDir:      e.InjectionDir,
		Platform:          e.Game.Store.String(),
		Store:             e.Game.Store,
		AppName:           e.Game.AppName,
		ExePath:           e.Game.ExePath,
		CompatPrefix:      e.Game.CompatPrefix,
		OptiScalerVersion: e.OptiScalerVersion,
		Status:            e.Status,
		Actionable:        actionableStatus(e.Status),
		Disabled:          e.Disabled,
		EAC:               e.EAC,
		ModTime:           e.ModTime,
		SteamAppID:        e.Game.SteamAppID,
		TitleSource:       string(e.Game.TitleSource),
	}
	keys := make([]string, 0, len(e.ComponentVersions))
	for k := range e.ComponentVersions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row.Components = append(row.Components, e.ComponentVersions[k])
	}
	for _, tech := range e.Tech {
		row.TechBadges = append(row.TechBadges, badgeForTech(tech))
	}
	if s.deps.Covers != nil {
		coverAppID := e.Game.AppID
		if e.Game.SteamAppID != "" {
			coverAppID = e.Game.SteamAppID
		}
		if !isNumericAppID(coverAppID) {
			// "custom_<folder>" manual ids carry no Steam meaning; digits
			// in a folder name ("Hades 2" → "2") would fetch a wrong
			// game's art and poison the miss cache with a bogus key.
			coverAppID = ""
		}
		if p, err := s.deps.Covers.Cover(ctx, coverAppID, e.Game.Name); err == nil {
			row.CoverPath = p
		}
	}
	return row
}

// refreshCovers rebinds cover art after the online identification phase
// has finalized titles and appids: rows with a resolved Steam appid get
// art for THAT appid (straight from the CDN), so a cover fetched for a
// codename title is replaced by the correct game's art the same scan.
// Rows whose identification produced a canonical title but no appid retry
// the search with that resolved title — the raw pre-identification query
// (folder or codename) is the weak one.
func (s *Session) refreshCovers(ctx context.Context, rows []GameRow) {
	if s.deps.Covers == nil {
		return
	}
	for i := range rows {
		if rows[i].SteamAppID == "" {
			// No appid: retry by the resolved title when the row has no
			// real art yet (.img files are real; the placeholder is not).
			// ponytail: the covers store search has no negative cache, so
			// an unresolvable title costs one live request per scan;
			// upgrade path: route the query through the steam client's
			// cached search.
			if rows[i].Title == "" || strings.HasSuffix(rows[i].CoverPath, ".img") {
				continue
			}
			if p, err := s.deps.Covers.Cover(ctx, "", rows[i].Title); err == nil {
				rows[i].CoverPath = p
			}
			continue
		}
		if strings.HasSuffix(rows[i].CoverPath, rows[i].SteamAppID+".img") {
			continue
		}
		if p, err := s.deps.Covers.Cover(ctx, rows[i].SteamAppID, rows[i].Title); err == nil {
			rows[i].CoverPath = p
		}
	}
}
