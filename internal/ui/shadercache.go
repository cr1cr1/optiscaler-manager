package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/shadercache"
)

// Shader-cache compilation management, per game (issue 078): discovery
// memo over the shadercache package, clear (claimed files now, shared
// driver caches behind consent), env presets over the existing env pin,
// and launch-and-record warming behind the CompilerBackend seam.

// shaderMemo caches one game's discovery; mutations (clear, warm,
// rescan) invalidate it, and Warm always measures fresh.
func (s *Session) shaderInput(row *GameRow) shadercache.Input {
	prefix := row.CompatPrefix
	if prefix == "" {
		prefix = s.UmuPrefixFor(row.InstallDir)
	}
	home, _ := s.homeDir()
	return shadercache.Input{
		GameDir:    row.InstallDir,
		PrefixDir:  prefix,
		HomeDir:    home,
		SteamRoot:  s.deps.SteamRoot,
		SteamAppID: row.SteamAppID,
	}
}

// ShaderCaches returns the memoized discovery for one game (nil for an
// unknown game). The pane renders from this every frame; the walk itself
// runs at most once per mutation.
func (s *Session) ShaderCaches(gameDir string) []shadercache.Ref {
	row := s.findRow(gameDir)
	if row == nil {
		return nil
	}
	s.mu.Lock()
	memo, ok := s.shaderMemos[gameDir]
	s.mu.Unlock()
	if ok {
		return memo
	}
	refs := shadercache.Discover(s.shaderInput(row))
	s.mu.Lock()
	s.shaderMemos[gameDir] = refs
	s.mu.Unlock()
	return refs
}

func (s *Session) invalidateShaderCache(gameDir string) {
	s.mu.Lock()
	delete(s.shaderMemos, gameDir)
	s.mu.Unlock()
}

func (s *Session) invalidateAllShaderCaches() {
	s.mu.Lock()
	s.shaderMemos = map[string][]shadercache.Ref{}
	s.mu.Unlock()
}

// freshShaderRefs measures now (never memoized): guards and Watchdog
// must see the bytes on disk, not the pane's snapshot.
func (s *Session) freshShaderRefs(row *GameRow) []shadercache.Ref {
	return shadercache.Discover(s.shaderInput(row))
}

// ClearShaderCache removes one tracked cache by path. Claimed per-game
// files go now; Steam precache and unknown-format dirs are refused
// (never ours to delete); shared driver caches pause on the consent
// gate. Anything untracked is a warning, never a deletion.
func (s *Session) ClearShaderCache(gameDir, path string) {
	row := s.findRow(gameDir)
	if row == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	var ref *shadercache.Ref
	refs := s.freshShaderRefs(row)
	for i := range refs {
		if refs[i].Path == path {
			ref = &refs[i]
			break
		}
	}
	if ref == nil {
		s.toast("not a tracked shader cache: "+path, true)
		return
	}
	switch ref.Kind {
	case shadercache.KindDXVK, shadercache.KindVKD3D:
		if err := s.removeAll(path); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("shader clear failed")
			s.toast("clear failed: "+err.Error(), true)
			return
		}
		s.invalidateShaderCache(gameDir)
		done := fmt.Sprintf("Cleared shader cache (%s)", formatBytes(ref.SizeBytes))
		s.setStatusGame(gameDir, done)
		s.toast(done, false)
		s.emit(Event{Kind: EvOpDone, Text: done, GameDir: gameDir})
	case shadercache.KindSteamFoz:
		s.toast("Steam owns the precache — use Steam's shader settings", true)
	case shadercache.KindUnclaimed:
		s.toast("not managed: unknown cache format, kept in place", true)
	case shadercache.KindMesa, shadercache.KindNvidia:
		s.setConfirm(&Confirmation{
			Kind:      ConfirmSharedCache,
			GameDir:   gameDir,
			clearPath: path,
			Message: fmt.Sprintf("This cache is shared by ALL games (%s). Clear it anyway?",
				formatBytes(ref.SizeBytes)),
		})
	}
}

// ClearGameShaderCaches drops every claimed per-game cache file (never
// precache, unknown-format dirs, or shared driver caches) and reports
// the freed bytes. One stubborn file warns and the rest still go.
func (s *Session) ClearGameShaderCaches(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	var cleared int
	var freed int64
	for _, r := range s.freshShaderRefs(row) {
		if r.Kind != shadercache.KindDXVK && r.Kind != shadercache.KindVKD3D {
			continue
		}
		if err := s.removeAll(r.Path); err != nil {
			log.Warn().Err(err).Str("path", r.Path).Msg("shader clear-all skipped a file")
			s.toast("clear skipped "+filepath.Base(r.Path)+": "+err.Error(), true)
			continue
		}
		cleared++
		freed += r.SizeBytes
	}
	s.invalidateShaderCache(gameDir)
	done := fmt.Sprintf("Cleared %d shader caches (%s)", cleared, formatBytes(freed))
	s.setStatusGame(gameDir, done)
	s.toast(done, false)
	s.emit(Event{Kind: EvOpDone, Text: done, GameDir: gameDir})
}

// clearSharedCache runs the consented global clear inside the op shell:
// a multi-gigabyte GLCache takes seconds, and the Working indicator
// plus double-run guard come free.
func (s *Session) clearSharedCache(gameDir, path string) {
	_, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Clearing shared shader cache…")
	err := s.removeAll(path)
	s.finishOp(gameDir)
	if err != nil {
		s.opFailed(err, gameDir)
		return
	}
	s.invalidateShaderCache(gameDir)
	s.opDone("Cleared shared shader cache", gameDir)
}

// EnvPreset is one KISS shader-cache env knob: an on/off toggle when
// Options is empty, a value selector otherwise ("default" clears the
// key back to the driver default).
type EnvPreset struct {
	Key     string
	Label   string
	On      string
	Off     string
	Options []string
}

// ShaderCacheEnvPresets lists the v1 knobs: the two disk-cache toggles
// plus Mesa's size cap (the only documented size knob). Path-redirect
// vars stay out by design — they would scatter caches past discovery.
func ShaderCacheEnvPresets() []EnvPreset {
	return []EnvPreset{
		{Key: "DXVK_STATE_CACHE", Label: "DXVK state cache", On: "1", Off: "0"},
		{Key: "__GL_SHADER_DISK_CACHE", Label: "NVIDIA disk cache", On: "1", Off: "0"},
		{Key: "MESA_SHADER_CACHE_MAX_SIZE", Label: "Mesa cache max size",
			Options: []string{"default", "1G", "5G", "10G"}},
	}
}

// SetShaderEnv writes one preset through the existing per-game env pin
// (the env editor keeps showing the raw values). Empty or "default"
// removes the key; toggling off never deletes caches, it only stops
// growth — the pane says so next to the toggle.
func (s *Session) SetShaderEnv(gameDir, key, value string) {
	dir := canonicalDir(gameDir)
	if s.findRow(dir) == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	env := s.UmuEnvFor(dir)
	if value == "" || value == "default" {
		delete(env, key)
	} else {
		env[key] = value
	}
	s.SetUmuEnvOverride(dir, env)
}

// Stop reasons for a warm run: what the watchdog actually observed.
const (
	// StopQuiet: cache sizes went still — the game settled.
	StopQuiet = "settled"
	// StopTimeout: the time cap hit with bytes still growing.
	StopTimeout = "time cap"
	// StopCancelled: the op was cancelled; partial gains stay.
	StopCancelled = "cancelled"
	// StopGameExited: the game exited on its own mid-run.
	StopGameExited = "game exited"
)

// WarmResult is what one compile run achieved, in the only units Linux
// truthfully offers: bytes grown and wall time.
type WarmResult struct {
	BytesGrown int64
	Elapsed    time.Duration
	StopReason string
}

// WarmTarget selects the game a backend warms.
type WarmTarget struct {
	Dir string
}

// CompilerBackend is the seam behind the Warm button (issue 078 §6): v1
// ships LaunchWatchBackend; a fossilize-replay backend reports the same
// result with its own stop reasons once validated on host hardware.
type CompilerBackend interface {
	Warm(ctx context.Context, target WarmTarget, onProgress func(grown int64, elapsed time.Duration)) (WarmResult, error)
}

// LaunchWatchBackend warms by launching the game through the existing
// route and watching the claimed cache sizes until they go quiet, the
// cap hits, or the game exits. No pipeline counts exist on Linux —
// quiescence is the completion signal.
type LaunchWatchBackend struct {
	Launch        func(dir string)
	Stop          func(dir string) bool
	Launched      func(dir string) bool
	Measure       func(dir string) int64
	Poll          time.Duration
	QuietNeeded   int
	MaxTime       time.Duration
	LaunchTimeout time.Duration
}

func (b LaunchWatchBackend) defaults() LaunchWatchBackend {
	if b.Poll <= 0 {
		b.Poll = 5 * time.Second
	}
	if b.QuietNeeded <= 0 {
		b.QuietNeeded = 3
	}
	if b.MaxTime <= 0 {
		b.MaxTime = 20 * time.Minute
	}
	if b.LaunchTimeout <= 0 {
		b.LaunchTimeout = time.Minute
	}
	return b
}

// Warm runs the launch-and-record loop. The game is left running for
// the caller to stop: every return path reports what to do, and the
// session wrapper always stops the session it started.
func (b LaunchWatchBackend) Warm(ctx context.Context, target WarmTarget, onProgress func(int64, time.Duration)) (WarmResult, error) {
	b = b.defaults()
	start := b.Measure(target.Dir)
	t0 := time.Now()
	elapsed := func() time.Duration { return time.Since(t0) }
	b.Launch(target.Dir)
	launched := false
	for deadline := time.Now().Add(b.LaunchTimeout); time.Now().Before(deadline); {
		if ctx.Err() != nil {
			return WarmResult{StopReason: StopCancelled}, ctx.Err()
		}
		if b.Launched(target.Dir) {
			launched = true
			break
		}
		if !sleepCtx(ctx, b.Poll/4+250*time.Millisecond) {
			return WarmResult{StopReason: StopCancelled}, ctx.Err()
		}
	}
	if !launched {
		return WarmResult{}, errors.New("launched game never reported running")
	}
	report := func() (grown int64, cur int64) {
		cur = b.Measure(target.Dir)
		if grown = cur - start; grown < 0 {
			grown = 0 // cleared mid-run: count from the new floor
		}
		return grown, cur
	}
	last := start
	quiet := 0
	for deadline := time.Now().Add(b.MaxTime); time.Now().Before(deadline); {
		if !sleepCtx(ctx, b.Poll) {
			grown, _ := report()
			return WarmResult{BytesGrown: grown, Elapsed: elapsed(), StopReason: StopCancelled}, ctx.Err()
		}
		grown, cur := report()
		onProgress(grown, elapsed())
		if !b.Launched(target.Dir) {
			return WarmResult{BytesGrown: grown, Elapsed: elapsed(), StopReason: StopGameExited}, nil
		}
		if cur != last {
			last, quiet = cur, 0
			continue
		}
		if quiet++; quiet >= b.QuietNeeded {
			return WarmResult{BytesGrown: grown, Elapsed: elapsed(), StopReason: StopQuiet}, nil
		}
	}
	grown, _ := report()
	return WarmResult{BytesGrown: grown, Elapsed: elapsed(), StopReason: StopTimeout}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// WarmProgress is the live snapshot WarmState reports while a run owns
// the game.
type WarmProgress struct {
	BytesGrown int64
	Elapsed    time.Duration
	Since      time.Time
}

// WarmState reports whether a warm run owns gameDir right now.
func (s *Session) WarmState(gameDir string) (WarmProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.warmRuns[gameDir]
	if !ok {
		return WarmProgress{}, false
	}
	return WarmProgress{BytesGrown: p.grown, Elapsed: time.Since(p.since), Since: p.since}, true
}

type warmRun struct {
	since time.Time
	grown int64
}

// warmBytesForWatch sums the per-game caches a watchdog may watch grow:
// claimed files plus the engine-owned dirs (growth there is warming
// evidence too — we never delete them, only observe). Steam precache
// and shared driver blobs are excluded.
func (s *Session) warmBytesForWatch(row *GameRow) (int64, bool) {
	var total int64
	trackable := false
	for _, r := range s.freshShaderRefs(row) {
		switch r.Kind {
		case shadercache.KindDXVK, shadercache.KindVKD3D, shadercache.KindUnclaimed:
			trackable = true
			total += r.SizeBytes
		}
	}
	return total, trackable
}

// StartWarm begins a launch-and-record run: guards first (unknown,
// unsupervised route, running game, concurrent warm, nothing
// trackable), then the backend inside the op shell. The run always
// stops the session it started — completion, cap, crash, or cancel.
func (s *Session) StartWarm(gameDir string) {
	dir := canonicalDir(gameDir)
	row := s.findRow(dir)
	if row == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	supervised := s.warmSupervised
	if supervised == nil {
		supervised = s.shouldRouteWinBinary
	}
	if !supervised(row) {
		s.toast("warm needs a supervised launch (umu/runner) — this game launches fire-and-forget", true)
		return
	}
	if s.LaunchBusy(dir) {
		s.toast(row.Title+" is already running", true)
		return
	}
	s.mu.Lock()
	_, warming := s.warmRuns[dir]
	s.mu.Unlock()
	if warming {
		s.toast("warm already in progress for "+row.Title, true)
		return
	}
	if _, trackable := s.warmBytesForWatch(row); !trackable {
		s.toast("no trackable shader cache for "+row.Title, true)
		return
	}
	backend := s.warmBackend
	if backend == nil {
		backend = LaunchWatchBackend{
			Launch:   s.Launch,
			Stop:     s.CancelOp,
			Launched: s.LaunchBusy,
			Measure: func(d string) int64 {
				r := s.findRow(d)
				if r == nil {
					return 0
				}
				total, _ := s.warmBytesForWatch(r)
				return total
			},
		}
	}
	stop := s.warmStop
	if stop == nil {
		stop = s.CancelOp
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(dir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.mu.Lock()
	s.warmRuns[dir] = &warmRun{since: s.now()}
	s.mu.Unlock()
	go func() {
		s.opStarted("Warming shaders…")
		res, err := backend.Warm(ctx, WarmTarget{Dir: dir}, func(grown int64, _ time.Duration) {
			s.mu.Lock()
			if p, ok := s.warmRuns[dir]; ok {
				p.grown = grown
			}
			s.mu.Unlock()
		})
		s.finishOp(dir)
		stopped := stop(dir)
		if stopped {
			log.Info().Str("gameDir", dir).Msg("warm stopped the launched game")
		}
		s.mu.Lock()
		delete(s.warmRuns, dir)
		s.mu.Unlock()
		switch {
		case errors.Is(err, context.Canceled):
			s.opCancelled(dir, pre)
		case err != nil:
			s.opFailed(err, dir)
		default:
			s.invalidateShaderCache(dir)
			s.opDone(fmt.Sprintf("Warmed: +%s in %s (%s)",
				formatBytes(res.BytesGrown), formatDuration(res.Elapsed), res.StopReason), dir)
		}
	}()
}

// formatBytes renders a byte count in the pane's units.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatDuration renders M:SS for warm toasts.
func formatDuration(d time.Duration) string {
	s := int64(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
