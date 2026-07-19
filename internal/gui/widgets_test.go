package gui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// headlessFrames prepares shirei for direct RunFrameFn driving so tests can
// inject keyboard input between frames (RenderToPNG resets input state on
// every call, so it cannot drive multi-frame focus interactions).
func headlessFrames(t *testing.T, w, h int) {
	t.Helper()
	WindowSize = Vec2{float32(w), float32(h)}
	WindowScale = 1
	if GlyphCacheBudgetBytes == 0 {
		GlyphCacheBudgetBytes = 16 << 20
	}
	HeadlessRender = true
	t.Cleanup(func() { HeadlessRender = false })
	ResetInputSession()
}

// keyFrame runs one frame with the given key press injected.
func keyFrame(key KeyCode, mod Modifiers, fn FrameFn) {
	InputState.Modifiers = mod
	FrameInput.Key = key
	RunFrameFn(fn)
	InputState.Modifiers = 0
}

func TestFocusableButtonTabCyclesAndEnterActivates(t *testing.T) {
	headlessFrames(t, 400, 200)
	var fired []string
	view := func() {
		Container(Attrs(Viewport), func() {
			if focusableButton(0, "Alpha") {
				fired = append(fired, "Alpha")
			}
			if focusableButton(0, "Beta") {
				fired = append(fired, "Beta")
			}
			if focusableButton(0, "Gamma") {
				fired = append(fired, "Gamma")
			}
		})
	}

	keyFrame(KeyCodeNone, 0, view)       // build + register focusables
	keyFrame(KeyTab, 0, view)            // nothing focused -> first button
	keyFrame(KeyEnter, 0, view)          // Enter activates Alpha
	keyFrame(KeyTab, 0, view)            // Alpha -> Beta
	keyFrame(KeySpace, 0, view)          // Space activates Beta
	keyFrame(KeyTab, ModShift, view)     // Beta -> Alpha
	keyFrame(KeyTab, ModShift, view)     // Alpha -> wraps back to Gamma
	keyFrame(KeyEnter, 0, view)          // Enter activates Gamma

	want := []string{"Alpha", "Beta", "Gamma"}
	if !slices.Equal(fired, want) {
		t.Fatalf("activation order %v, want %v (tab cycle + enter/space)", fired, want)
	}
	t.Logf("tab cycle + activation order: %v", fired)
}

func TestFocusableButtonConsumesKey(t *testing.T) {
	headlessFrames(t, 300, 120)
	var fired bool
	var leaked KeyCode
	var armed bool
	view := func() {
		Container(Attrs(Viewport), func() {
			if focusableButton(0, "Go") {
				fired = true
			}
			// Probe: any widget rendered after the focused button in the
			// same frame must observe the activation key as consumed.
			if armed && FrameInput.Key != KeyCodeNone {
				leaked = FrameInput.Key
			}
		})
	}

	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyTab, 0, view)
	armed = true
	keyFrame(KeyEnter, 0, view)

	if !fired {
		t.Error("Enter on the focused button did not activate it")
	}
	if leaked != KeyCodeNone {
		t.Errorf("activation key leaked past FocusableButton (code %d): later widgets could double-fire", leaked)
	}
	t.Log("Enter activated once and was consumed")
}

func TestExitButtonFlushesSettings(t *testing.T) {
	root := t.TempDir()
	sess := ui.NewSession(ui.Deps{SettingsRoot: root, Settings: settings.Defaults()})
	m := newModel(Config{Session: sess})

	exitCode := -1
	flushedAtExit := ""
	m.exitNow = func(code int) {
		exitCode = code
		s, err := settings.Load(root)
		if err != nil {
			t.Errorf("settings unreadable at exit time: %v", err)
			return
		}
		flushedAtExit = s.DefaultVersion
	}
	m.versionBuf = "v9.9.9-test" // pending edit in the settings modal

	m.exit()

	if exitCode != 0 {
		t.Errorf("exit seam code %d, want 0", exitCode)
	}
	if flushedAtExit != "v9.9.9-test" {
		t.Errorf("default version at exit time %q, want flushed %q", flushedAtExit, "v9.9.9-test")
	}
	// A nil session must still exit cleanly.
	m2 := newModel(Config{})
	m2.exitNow = func(code int) { exitCode = code }
	m2.exit()
	if exitCode != 0 {
		t.Errorf("session-less exit code %d, want 0", exitCode)
	}
	t.Logf("exit flushed settings (default version %q) before quit", flushedAtExit)
}

func TestCardShowsVersionBadges(t *testing.T) {
	e := &ui.GameRow{
		Status:            domain.StatusCommitted,
		OptiScalerVersion: "0.9.4",
		Components:        []string{"DLSS 3.7.10", "FSR 3.1.4"},
		CompatPrefix:      "/pfx/123",
	}
	pills := versionPills(e)
	var labels []string
	for _, p := range pills {
		labels = append(labels, p.Label)
	}
	want := []string{"✦ OptiScaler 0.9.4", "DLSS 3.7.10", "FSR 3.1.4", "Proton"}
	if !slices.Equal(labels, want) {
		t.Fatalf("version pills %v, want %v", labels, want)
	}
	tones := []ui.Tone{ui.TonePurple, ui.ToneGreen, ui.ToneRed, ui.ToneBlue}
	for i, p := range pills {
		if p.Tone != tones[i] {
			t.Errorf("pill %q tone %v, want %v", p.Label, p.Tone, tones[i])
		}
	}

	plain := versionPills(&ui.GameRow{Status: domain.StatusCommitted})
	if len(plain) != 1 || plain[0].Label != "✦ OptiScaler" {
		t.Errorf("committed without known version: %v, want the plain OptiScaler pill", plain)
	}
	if got := versionPills(&ui.GameRow{}); len(got) != 0 {
		t.Errorf("uninstalled row pills %v, want none", got)
	}
	t.Logf("version badges: %v", labels)
}

// launchRecorder captures the argv handed to the injected launch runner.
type launchRecorder struct {
	mu    sync.Mutex
	calls int
	argv  []string
}

func (r *launchRecorder) runner() launch.Runner {
	return func(_ context.Context, _, name string, args ...string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls++
		r.argv = append([]string{name}, args...)
		return nil
	}
}

func (r *launchRecorder) captured() (int, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]string(nil), r.argv...)
}

func TestLaunchButtonCallsSessionLaunch(t *testing.T) {
	rec := &launchRecorder{}
	noBinaries := func(string) (string, error) { return "", errors.New("not found") }
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.Launcher = launch.New(rec.runner(), "linux", noBinaries)
	})
	m := newModel(Config{Session: sess})

	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) == 0 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	rows := sess.VisibleRows()
	if len(rows) != 1 {
		t.Fatalf("scanned rows %d, want 1", len(rows))
	}
	row := rows[0]
	if !launchable(&row) {
		t.Fatalf("scanned row %+v not launchable (AppID %q, ExePath %q)", row, row.AppID, row.ExePath)
	}

	m.launchGame(row)
	for time.Now().Before(deadline) {
		select {
		case ev := <-sess.Events():
			if ev.Kind == ui.EvOpDone {
				goto launched
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("no EvOpDone after Launch")
launched:
	calls, argv := rec.captured()
	if calls != 1 {
		t.Fatalf("runner calls %d, want 1", calls)
	}
	found := false
	for _, a := range argv {
		if a == "steam://rungameid/100" {
			found = true
		}
	}
	if !found {
		t.Errorf("captured argv %v lacks steam://rungameid/100", argv)
	}

	// Gating: a row with neither AppID nor ExePath offers no launch.
	if launchable(&ui.GameRow{Title: "Ghost"}) {
		t.Error("empty row reported launchable")
	}
	m.launchGame(ui.GameRow{Title: "Ghost", InstallDir: filepath.Join("nowhere", "ghost")})
	if calls, _ := rec.captured(); calls != 1 {
		t.Errorf("runner calls %d after non-launchable row, want still 1", calls)
	}
	t.Logf("launch argv: %v", argv)
}
