package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/shadercache"
)

// scanGameOne scans the fixture library; the caller gets the game dir.
func scanGameOne(t *testing.T, e *testEnv) string {
	t.Helper()
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	return e.gameRoot
}

// TestShaderCachesDiscovery: known files surface as claimed refs, and the
// result is memoized until a mutation invalidates it.
func TestShaderCachesDiscovery(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	writeUIFile(t, filepath.Join(e.bin, "game.dxvk-cache"), "DXVKDATA")

	refs := e.sess.ShaderCaches(dir)
	if len(refs) != 1 || refs[0].Kind != shadercache.KindDXVK {
		t.Fatalf("refs = %+v, want one DXVK claim", refs)
	}

	// Memoized: deleting behind the session's back changes nothing.
	if err := os.Remove(filepath.Join(e.bin, "game.dxvk-cache")); err != nil {
		t.Fatal(err)
	}
	if refs := e.sess.ShaderCaches(dir); len(refs) != 1 {
		t.Fatalf("memoized refs = %+v, want the stale claim", refs)
	}
	if st := e.sess.Snapshot(); st.Confirm != nil {
		t.Fatal("discovery raised a confirmation")
	}
}

// TestClearShaderCacheFile: a claimed file is removed; unclaimed bytes,
// unknown paths, and unknown games are refused with a warning.
func TestClearShaderCacheFile(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	dxvk := filepath.Join(e.bin, "game.dxvk-cache")
	writeUIFile(t, dxvk, "DXVKDATA")
	kept := filepath.Join(e.bin, "cache", "blob.bin")
	writeUIFile(t, kept, "ENGINE")

	e.sess.ClearShaderCache(dir, kept)
	if _, err := os.Stat(kept); err != nil {
		t.Fatal("unclaimed bytes removed without consent")
	}

	e.sess.ClearShaderCache(dir, filepath.Join(e.bin, "nope.dxvk-cache"))
	if _, err := os.Stat(dxvk); err != nil {
		t.Fatal("unknown path cleared the real cache")
	}

	e.sess.ClearShaderCache(filepath.Join(dir, "missing"), dxvk)
	if _, err := os.Stat(dxvk); err != nil {
		t.Fatal("unknown game cleared the real cache")
	}

	e.sess.ClearShaderCache(dir, dxvk)
	if _, err := os.Stat(dxvk); !os.IsNotExist(err) {
		t.Fatal("claimed cache file survived clearing")
	}
	// Clearing re-derives: the memo no longer reports the removed file.
	for _, r := range e.sess.ShaderCaches(dir) {
		if r.Path == dxvk {
			t.Fatalf("memo still reports %+v after clear", r)
		}
	}
}

// TestClearGameShaderCaches: clear-all drops every claimed per-game file
// and nothing else, reporting bytes.
func TestClearGameShaderCaches(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	writeUIFile(t, filepath.Join(e.bin, "a.dxvk-cache"), "1234")
	writeUIFile(t, filepath.Join(e.bin, "vkd3d-proton.cache"), "5678")
	writeUIFile(t, filepath.Join(e.bin, "cache", "engine.blob"), "90")

	e.sess.ClearGameShaderCaches(dir)
	for _, p := range []string{"a.dxvk-cache", "vkd3d-proton.cache"} {
		if _, err := os.Stat(filepath.Join(e.bin, p)); !os.IsNotExist(err) {
			t.Errorf("%s survived clear-all", p)
		}
	}
	if _, err := os.Stat(filepath.Join(e.bin, "cache", "engine.blob")); err != nil {
		t.Error("unclaimed engine bytes removed by clear-all")
	}
}

// TestClearSharedCacheConsent: a global driver cache pauses on the shared
// consent gate; declining keeps every byte, accepting clears it.
func TestClearSharedCacheConsent(t *testing.T) {
	e := newTestEnv(t)
	mesaFile := filepath.Join(e.home, ".cache", "mesa_shader_cache", "f")
	writeUIFile(t, mesaFile, "MESADATA")
	dir := scanGameOne(t, e)

	var mesaPath string
	for _, r := range e.sess.ShaderCaches(dir) {
		if r.Kind == shadercache.KindMesa {
			mesaPath = r.Path
		}
	}
	if mesaPath == "" {
		t.Fatal("no mesa ref discovered")
	}
	e.sess.ClearShaderCache(dir, mesaPath)
	ev := waitEvent(t, e.sess, EvConfirm)
	if !strings.Contains(ev.Text, "shared") {
		t.Errorf("confirm text %q does not warn the cache is shared", ev.Text)
	}
	if _, err := os.Stat(mesaFile); err != nil {
		t.Fatal("shared cache removed before consent")
	}
	e.sess.AnswerConfirm(false)
	if _, err := os.Stat(mesaFile); err != nil {
		t.Fatal("declined clear removed the shared cache")
	}
	e.sess.ClearShaderCache(dir, mesaPath)
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(mesaFile); !os.IsNotExist(err) {
		t.Fatal("consented clear kept the shared cache")
	}
}

// TestShaderEnvPresets: the KISS presets write through the existing
// per-game env pin; empty removes the key.
func TestShaderEnvPresets(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)

	var dxvk *EnvPreset
	for i, p := range ShaderCacheEnvPresets() {
		if p.Key == "DXVK_STATE_CACHE" {
			dxvk = &ShaderCacheEnvPresets()[i]
		}
	}
	if dxvk == nil || dxvk.On == "" || dxvk.Off == "" {
		t.Fatal("no DXVK on/off preset")
	}
	e.sess.SetShaderEnv(dir, dxvk.Key, dxvk.On)
	if got := e.sess.UmuEnvFor(dir)[dxvk.Key]; got != dxvk.On {
		t.Errorf("env %s = %q, want %q", dxvk.Key, got, dxvk.On)
	}
	e.sess.SetShaderEnv(dir, dxvk.Key, "")
	if _, ok := e.sess.UmuEnvFor(dir)[dxvk.Key]; ok {
		t.Error("empty value did not remove the key")
	}
	e.sess.SetShaderEnv(filepath.Join(dir, "missing"), dxvk.Key, dxvk.On)
}

// TestWarmMeasureIgnoresSharedTrees: the watchdog polls for up to 20
// minutes — its measurer must never pay for multi-gigabyte shared-tree
// walks it doesn't sum (review: home re-walk every 5s).
func TestWarmMeasureIgnoresSharedTrees(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	writeUIFile(t, filepath.Join(e.bin, "game.dxvk-cache"), string(make([]byte, 100)))
	writeUIFile(t, filepath.Join(e.home, ".cache", "nvidia", "GLCache", "g"), string(make([]byte, 200)))

	b := e.sess.newLaunchWatchBackend()
	if got := b.Measure(dir); got != 100 {
		t.Errorf("watch measure = %d, want the 100 game bytes only", got)
	}
}

// TestClearSharedCacheRevalidates: a consented clear re-checks the path
// is still a tracked shared cache when the answer lands — a rescan or a
// vanished dir between consent and accept must refuse, not delete.
func TestClearSharedCacheRevalidates(t *testing.T) {
	e := newTestEnv(t)
	mesaFile := filepath.Join(e.home, ".cache", "mesa_shader_cache", "f")
	writeUIFile(t, mesaFile, "MESADATA")
	dir := scanGameOne(t, e)
	var mesaPath string
	for _, r := range e.sess.ShaderCaches(dir) {
		if r.Kind == shadercache.KindMesa {
			mesaPath = r.Path
		}
	}
	e.sess.ClearShaderCache(dir, mesaPath)
	waitEvent(t, e.sess, EvConfirm)
	if err := os.RemoveAll(mesaPath); err != nil {
		t.Fatal(err)
	}
	e.sess.ShaderCaches(dir) // memo still names it; the answer must re-derive
	e.sess.AnswerConfirm(true)
	ev := waitEvent(t, e.sess, EvOpFailed)
	if !strings.Contains(ev.Text, "tracked") {
		t.Errorf("refusal text %q does not say untracked", ev.Text)
	}
}

// fakeBackend is a scripted CompilerBackend: it blocks until ctx ends
// (like a real game session) unless result is preset.
type fakeBackend struct {
	mu       sync.Mutex
	calls    int
	result   WarmResult
	progress []int64
	unblock  chan struct{}
}

func (f *fakeBackend) Warm(ctx context.Context, _ WarmTarget, onProgress func(int64, time.Duration)) (WarmResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.unblock != nil {
		<-ctx.Done()
		return WarmResult{StopReason: StopCancelled}, ctx.Err()
	}
	return f.result, nil
}

func (f *fakeBackend) started() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls > 0
}

// TestWarmGuards: warm refuses unknown games, running games, concurrent
// warms, and games with nothing trackable.
func TestWarmGuards(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	fb := &fakeBackend{result: WarmResult{BytesGrown: 512, StopReason: StopQuiet}}
	e.sess.warmBackend = fb
	e.sess.warmSupervised = func(*GameRow) bool { return true }

	e.sess.StartWarm(filepath.Join(dir, "missing"))
	if fb.started() {
		t.Fatal("warm started for an unknown game")
	}
	// Nothing trackable: no caches at all.
	e.sess.StartWarm(dir)
	if fb.started() {
		t.Fatal("warm started with no trackable cache")
	}

	writeUIFile(t, filepath.Join(e.bin, "game.dxvk-cache"), "DXVKDATA")

	// Fake a running launch: warm must not friendly-fire it.
	e.sess.launchRuns[dir] = func() {}
	e.sess.StartWarm(dir)
	if fb.started() {
		t.Fatal("warm started while the game was running")
	}
	delete(e.sess.launchRuns, dir)

	// Trackable cache present, nothing running: the backend runs.
	e.sess.StartWarm(dir)
	waitEvent(t, e.sess, EvOpDone)
	if !fb.started() {
		t.Fatal("warm did not start with a trackable cache")
	}
}

// TestWarmRunCompletes: a finished backend run settles the op shell and
// reports the grown bytes.
func TestWarmRunCompletes(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	writeUIFile(t, filepath.Join(e.bin, "game.dxvk-cache"), "DXVKDATA")
	fb := &fakeBackend{result: WarmResult{BytesGrown: 1 << 20, StopReason: StopQuiet}}
	e.sess.warmBackend = fb
	e.sess.warmSupervised = func(*GameRow) bool { return true }

	e.sess.StartWarm(dir)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "MB") {
		t.Errorf("warm done text %q does not report megabytes", ev.Text)
	}
	if _, running := e.sess.WarmState(dir); running {
		t.Error("warm still marked running after completion")
	}
	if !fb.started() {
		t.Error("backend never ran")
	}
}

// TestWarmCancelStopsGame: cancelling the op ends the backend and stops
// the launched game through the existing stop path.
func TestWarmCancelStopsGame(t *testing.T) {
	e := newTestEnv(t)
	dir := scanGameOne(t, e)
	writeUIFile(t, filepath.Join(e.bin, "game.dxvk-cache"), "DXVKDATA")
	fb := &fakeBackend{unblock: make(chan struct{})}
	e.sess.warmBackend = fb
	e.sess.warmSupervised = func(*GameRow) bool { return true }
	var stopped []string
	e.sess.warmStop = func(dir string) bool {
		stopped = append(stopped, dir)
		return true
	}
	e.sess.CancelOp(dir) // no-op before start, must not panic

	e.sess.StartWarm(dir)
	deadline := time.After(10 * time.Second)
	for {
		if _, running := e.sess.WarmState(dir); running {
			break
		}
		select {
		case <-deadline:
			t.Fatal("warm never reached running state")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !e.sess.CancelOp(dir) {
		t.Fatal("CancelOp reported nothing to cancel")
	}
	ev := waitEvent(t, e.sess, EvOpCancelled)
	if ev.GameDir != dir {
		t.Errorf("cancel event dir = %q, want %q", ev.GameDir, dir)
	}
	if _, running := e.sess.WarmState(dir); running {
		t.Error("warm still marked running after cancel")
	}
	if len(stopped) != 1 || stopped[0] != dir {
		t.Errorf("stopped = %v, want the game stopped once after cancel", stopped)
	}
}
