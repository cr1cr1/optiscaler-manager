package gui

import (
	"context"
	"errors"
	"os"
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

// comboFrame is keyFrame with the frame output returned (clipboard
// assertions need out.Copy/out.Paste).
func comboFrame(key KeyCode, mod Modifiers, fn FrameFn) FrameOutputData {
	InputState.Modifiers = mod
	FrameInput.Key = key
	out := RunFrameFn(fn)
	InputState.Modifiers = 0
	return out
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

	keyFrame(KeyCodeNone, 0, view)   // build + register focusables
	keyFrame(KeyTab, 0, view)        // nothing focused -> first button
	keyFrame(KeyEnter, 0, view)      // Enter activates Alpha
	keyFrame(KeyTab, 0, view)        // Alpha -> Beta
	keyFrame(KeySpace, 0, view)      // Space activates Beta
	keyFrame(KeyTab, ModShift, view) // Beta -> Alpha
	keyFrame(KeyTab, ModShift, view) // Alpha -> wraps back to Gamma
	keyFrame(KeyEnter, 0, view)      // Enter activates Gamma

	want := []string{"Alpha", "Beta", "Gamma"}
	if !slices.Equal(fired, want) {
		t.Fatalf("activation order %v, want %v (tab cycle + enter/space)", fired, want)
	}
	t.Logf("tab cycle + activation order: %v", fired)
}

// TestFocusableButtonClickFocuses: clicking a focusable button hands it
// keyboard focus (FocusOnClick) AND still activates it — grabbing focus
// must not eat the click's PressAction.
func TestFocusableButtonClickFocuses(t *testing.T) {
	headlessFrames(t, 400, 200)
	var fired []string
	var alphaID, betaID ContainerId
	var alphaRect Rect
	view := func() {
		Container(Attrs(Viewport), func() {
			if focusableButton(0, "Alpha") {
				fired = append(fired, "Alpha")
			}
			alphaID = GetLastId()
			alphaRect = GetScreenRectOf(alphaID)
			if focusableButton(0, "Beta") {
				fired = append(fired, "Beta")
			}
			betaID = GetLastId()
		})
	}

	keyFrame(KeyCodeNone, 0, view) // build + register focusables
	keyFrame(KeyCodeNone, 0, view) // screen rects resolve from the prior frame
	if alphaRect.Size[0] == 0 {
		t.Fatalf("Alpha button rect not resolved: %+v", alphaRect)
	}
	clickRect(alphaRect, view)
	if !IdHasFocus(alphaID) {
		t.Error("click did not focus the Alpha button")
	}
	if IdHasFocus(betaID) {
		t.Error("Beta took focus without being clicked")
	}
	if !slices.Equal(fired, []string{"Alpha"}) {
		t.Errorf("click activation %v, want [Alpha] (FocusOnClick must not eat the activation)", fired)
	}
	t.Log("click focused and activated the button")
}

// TestFocusableToggleClickFocuses: clicking a focusable toggle hands it
// keyboard focus (FocusOnClick) AND still flips the bound value via the
// switch itself.
func TestFocusableToggleClickFocuses(t *testing.T) {
	headlessFrames(t, 400, 200)
	on := false
	var toggleID ContainerId
	var toggleRect Rect
	view := func() {
		Container(Attrs(Viewport), func() {
			focusableToggle(&on, "Online lookups")
			toggleID = GetLastId()
			toggleRect = GetScreenRectOf(toggleID)
		})
	}

	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if toggleRect.Size[0] == 0 {
		t.Fatalf("toggle rect not resolved: %+v", toggleRect)
	}
	// Aim at the switch itself: it sits at the row's leading edge, left of
	// the label (the wrapper's center is over the label, which is inert).
	clickRect(Rect{Origin: toggleRect.Origin, Size: Vec2{24, toggleRect.Size[1]}}, view)
	if !IdHasFocus(toggleID) {
		t.Error("click did not focus the toggle")
	}
	if !on {
		t.Error("click did not flip the toggle on (FocusOnClick must not eat the switch click)")
	}
	t.Log("click focused and flipped the toggle")
}

// TestFocusableClickOutsideBlurs: with a control focused by a click, a
// click landing OUTSIDE it blurs it — FocusOnClick's second branch
// (focused && !hovered -> Blur).
func TestFocusableClickOutsideBlurs(t *testing.T) {
	headlessFrames(t, 400, 200)
	var btnID ContainerId
	var btnRect Rect
	view := func() {
		Container(Attrs(Viewport), func() {
			focusableButton(0, "Only")
			btnID = GetLastId()
			btnRect = GetScreenRectOf(btnID)
		})
	}

	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if btnRect.Size[0] == 0 {
		t.Fatalf("button rect not resolved: %+v", btnRect)
	}
	clickRect(btnRect, view)
	if !IdHasFocus(btnID) {
		t.Fatal("setup: click did not focus the button")
	}
	// Click empty space in the viewport's far corner.
	clickRect(Rect{Origin: Vec2{370, 170}, Size: Vec2{10, 10}}, view)
	if IdHasFocus(btnID) {
		t.Error("clicking outside the focused button did not blur it")
	}
	t.Log("click outside blurred the focused control")
}

// TestVersionDropdown_ClickFocusesTrigger: clicking the version-dropdown
// trigger hands it keyboard focus (FocusOnClick) AND still toggles the
// popup open.
func TestVersionDropdown_ClickFocusesTrigger(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	r := m.versionDDRects[row.InstallDir]
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("version dropdown trigger not rendered for %q (rect %+v)", row.InstallDir, r)
	}
	clickRect(r, view)
	keyFrame(KeyCodeNone, 0, view) // popup frame settles
	if !IdHasFocus(m.ddTriggerID) {
		t.Error("click did not focus the version-dropdown trigger")
	}
	if m.versionDDItemsFor != row.InstallDir {
		t.Error("click did not open the dropdown (FocusOnClick must not eat the activation)")
	}
	t.Log("click focused the trigger and opened the dropdown")
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

// `/` focuses the search field from anywhere; typed text then lands in it.
func TestGUISlashFocusesSearch(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	dir := filepath.Join(t.TempDir(), "GameA")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZGAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.AddDirectory(dir)
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) < 1 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if len(sess.VisibleRows()) != 1 {
		t.Fatalf("rows %d, want 1", len(sess.VisibleRows()))
	}

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.searchID
	FrameInput.Text = "/"
	RunFrameFn(m.rootView)
	FrameInput.Text = "ab"
	RunFrameFn(m.rootView)
	if m.filter != "ab" {
		t.Errorf("filter = %q, want %q (/ focused the search and typing landed in it)", m.filter, "ab")
	}
}

// Cards with and without pill rows pin their buttons to the same bottom Y.
func TestGUICardButtonsBottomAligned(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.cardW = 200
	m.cardH = cardContentH(m.cardW)
	withPills := ui.GameRow{Title: "Pilled", Status: domain.StatusCommitted, Components: []string{"DLSS 4.0"}}
	bare := ui.GameRow{Title: "Bare"}

	headlessFrames(t, 800, 600)
	view := func() {
		Container(Attrs(Viewport, Row), func() {
			m.gameCard(withPills)
			m.gameCard(bare)
		})
	}
	RunFrameFn(view) // build + resolve layout
	var yA, yB float32
	RunFrameFn(func() {
		Container(Attrs(Viewport, Row), func() {
			m.gameCard(withPills)
			yA = m.cardBtnRect.Origin[1]
			m.gameCard(bare)
			yB = m.cardBtnRect.Origin[1]
		})
	})
	if yA != yB {
		t.Errorf("button row Y = %v vs %v, want identical bottom alignment", yA, yB)
	}
	want := float32(m.cardH - buttonRowH)
	if yA != want {
		t.Errorf("button row Y = %v, want %v (pinned to the card bottom)", yA, want)
	}
}

// The sidebar shell fills the full window height.
func TestGUISidebarFullHeight(t *testing.T) {
	m := newModel(Config{})
	headlessFrames(t, 800, 600)
	RunFrameFn(m.rootView)
	if got := m.sidebarShellRect.Size[1]; got != 600 {
		t.Errorf("sidebar height = %v, want the full 600", got)
	}
}

// editField renders one themedInput bound to buf with an explicit edit
// state and returns it (focus applied), so tests assert on st directly.
func editField(t *testing.T, buf *string) (*editState, FrameFn) {
	t.Helper()
	st := &editState{cursor: len([]rune(*buf)), anchor: -1, blink: time.Now(), phase: true}
	var searchID ContainerId
	view := func() {
		Container(Attrs(Viewport, Pad(8)), func() {
			themedInputState(buf, "Search…", 0, st, Grow(1), MinSize(140, fieldH), MaxSizeVec(Vec2{420, fieldH}))
			searchID = GetLastId()
		})
	}
	headlessFrames(t, 460, 60)
	RunFrameFn(view)
	FocusImmediateOn(searchID)
	RunFrameFn(view)
	return st, view
}

func textFrame(text string, fn FrameFn) {
	FrameInput.Text = text
	RunFrameFn(fn)
}

func TestEditCursorMotionAndInsert(t *testing.T) {
	buf := ""
	st, view := editField(t, &buf)
	textFrame("abcd", view)
	keyFrame(KeyLeft, 0, view)
	keyFrame(KeyLeft, 0, view)
	if st.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 after two Left", st.cursor)
	}
	textFrame("X", view)
	if buf != "abXcd" {
		t.Errorf("buf = %q, want %q (insert at cursor)", buf, "abXcd")
	}
	keyFrame(KeyHome, 0, view)
	textFrame("Y", view)
	keyFrame(KeyEnd, 0, view)
	textFrame("Z", view)
	if buf != "YabXcdZ" {
		t.Errorf("buf = %q, want %q (Home/End insertion)", buf, "YabXcdZ")
	}
}

func TestEditShiftSelectAndReplace(t *testing.T) {
	buf := ""
	st, view := editField(t, &buf)
	textFrame("abcd", view)
	keyFrame(KeyHome, 0, view)
	keyFrame(KeyRight, ModShift, view)
	keyFrame(KeyRight, ModShift, view)
	if st.anchor != 0 || st.cursor != 2 {
		t.Fatalf("anchor=%d cursor=%d, want 0/2 (shift-selected 'ab')", st.anchor, st.cursor)
	}
	textFrame("X", view)
	if buf != "Xcd" {
		t.Errorf("buf = %q, want %q (selection replaced)", buf, "Xcd")
	}
	if st.anchor != -1 {
		t.Errorf("anchor = %d, want -1 (selection collapsed after replace)", st.anchor)
	}
}

func TestEditSelectAllCopyCutPaste(t *testing.T) {
	buf := "hello"
	st, view := editField(t, &buf)
	keyFrame(KeyA, ModCtrl, view)
	if st.anchor != 0 || st.cursor != 5 {
		t.Fatalf("anchor=%d cursor=%d, want 0/5 (select all)", st.anchor, st.cursor)
	}
	out := comboFrame(KeyC, ModCtrl, view)
	if out.Copy != "hello" {
		t.Errorf("Copy = %q, want %q", out.Copy, "hello")
	}
	out = comboFrame(KeyX, ModCtrl, view)
	if out.Copy != "hello" || buf != "" {
		t.Errorf("after cut: Copy = %q buf = %q, want %q/''", out.Copy, buf, "hello")
	}
	out = comboFrame(KeyV, ModCtrl, view)
	if !out.Paste {
		t.Error("Paste not requested after Ctrl+V")
	}
	textFrame("hello", view) // paste arrives as text next frame
	if buf != "hello" {
		t.Errorf("buf = %q after paste, want %q", buf, "hello")
	}
}

func TestEditBackspaceDeletesSelection(t *testing.T) {
	buf := "hello"
	st, view := editField(t, &buf)
	keyFrame(KeyA, ModCtrl, view)
	keyFrame(KeyDeleteBackward, 0, view)
	if buf != "" {
		t.Errorf("buf = %q, want empty (selection deleted)", buf)
	}
	if st.cursor != 0 || st.anchor != -1 {
		t.Errorf("cursor=%d anchor=%d, want 0/-1", st.cursor, st.anchor)
	}
	// A plain backspace at the end deletes one rune.
	textFrame("ab", view)
	keyFrame(KeyDeleteBackward, 0, view)
	if buf != "a" {
		t.Errorf("buf = %q, want %q (single-rune delete)", buf, "a")
	}
}

// mouseFrame injects one frame of mouse input at (x, y): action is
// MouseClick, MouseRelease, or 0 for plain motion/hover.
func mouseFrame(x, y float32, action MouseAction, fn FrameFn) {
	InputState.MousePoint = Vec2{x, y}
	FrameInput.Mouse = action
	RunFrameFn(fn)
	FrameInput.Mouse = 0
}

// clickX returns the screen x of the caret at rune index idx inside st's
// recorded text area (with a nudge right so the midpoint rule lands on idx).
func clickX(st *editState, text string, idx int) float32 {
	r := []rune(text)
	return st.textRect.Origin[0] + textWidth(string(r[:idx])) + 0.01
}

func TestHitIndex(t *testing.T) {
	if got := hitIndex("", 10); got != 0 {
		t.Errorf("hitIndex('', 10) = %d, want 0", got)
	}
	if got := hitIndex("hello", -3); got != 0 {
		t.Errorf("hitIndex(hello, -3) = %d, want 0 (clamped left)", got)
	}
	if got := hitIndex("hello", textWidth("hello")+5); got != 5 {
		t.Errorf("hitIndex past end = %d, want 5 (clamped right)", got)
	}
	if got := hitIndex("hello", textWidth("hel")+0.01); got != 3 {
		t.Errorf("hitIndex at 'hel|' = %d, want 3", got)
	}
	// The midpoint of a glyph decides: left half maps before, right half after.
	mid := textWidth("he") + (textWidth("hel")-textWidth("he"))/2
	if got := hitIndex("hello", mid-0.01); got != 2 {
		t.Errorf("hitIndex left of midpoint = %d, want 2", got)
	}
	if got := hitIndex("hello", mid+0.01); got != 3 {
		t.Errorf("hitIndex right of midpoint = %d, want 3", got)
	}
	// Multibyte runes: glyph clusters must be rune indices, not byte offsets.
	if got := hitIndex("héllo", textWidth("hé")+0.01); got != 2 {
		t.Errorf("hitIndex('héllo', after é) = %d, want 2 (rune index)", got)
	}
}

// A plain click moves the caret to the clicked glyph and drops any selection.
func TestEditMouseClickPositionsCaret(t *testing.T) {
	buf := "hello world"
	st, view := editField(t, &buf)
	x := clickX(st, buf, 3)
	mouseFrame(x, 20, 0, view) // register hover at the click point
	mouseFrame(x, 20, MouseClick, view)
	mouseFrame(x, 20, MouseRelease, view)
	if st.cursor != 3 || st.anchor != -1 {
		t.Errorf("cursor=%d anchor=%d, want 3/-1 (caret at clicked glyph, no selection)", st.cursor, st.anchor)
	}
}

// Press + drag + release selects the dragged range.
func TestEditMouseDragSelects(t *testing.T) {
	buf := "hello world"
	st, view := editField(t, &buf)
	x2, x7 := clickX(st, buf, 2), clickX(st, buf, 7)
	mouseFrame(x2, 20, 0, view)
	mouseFrame(x2, 20, MouseClick, view)
	mouseFrame(x7, 20, 0, view) // drag motion with the button held
	mouseFrame(x7, 20, MouseRelease, view)
	lo, hi, has := st.selRange(len([]rune(buf)))
	if !has || lo != 2 || hi != 7 {
		t.Errorf("selection = (%d,%d,%v), want (2,7,true) after drag", lo, hi, has)
	}
	if st.dragging {
		t.Error("dragging still set after MouseRelease")
	}
}

// Shift+click extends the selection from the caret to the clicked glyph.
func TestEditMouseShiftClickExtends(t *testing.T) {
	buf := "hello world"
	st, view := editField(t, &buf)
	st.cursor = 2
	x := clickX(st, buf, 7)
	InputState.Modifiers = ModShift
	mouseFrame(x, 20, 0, view)
	mouseFrame(x, 20, MouseClick, view)
	InputState.Modifiers = 0
	lo, hi, has := st.selRange(len([]rune(buf)))
	if !has || lo != 2 || hi != 7 {
		t.Errorf("selection = (%d,%d,%v), want (2,7,true) after shift+click", lo, hi, has)
	}
}

// Double-click selects the word under the cursor.
func TestEditMouseDoubleClickSelectsWord(t *testing.T) {
	buf := "hello world"
	st, view := editField(t, &buf)
	x := clickX(st, buf, 8) // inside "world"
	mouseFrame(x, 20, 0, view)
	mouseFrame(x, 20, MouseClick, view)
	mouseFrame(x, 20, MouseRelease, view)
	mouseFrame(x, 20, MouseClick, view) // second click of the streak
	lo, hi, has := st.selRange(len([]rune(buf)))
	if !has || lo != 6 || hi != 11 {
		t.Errorf("selection = (%d,%d,%v), want (6,11,true) — the word 'world'", lo, hi, has)
	}
}

// The text row keeps identical geometry across hint, focused-caret, and
// text states: the caret must not change the row height (click-jitter).
func TestEditFieldRowGeometryStable(t *testing.T) {
	buf := ""
	st, view := editField(t, &buf) // focused + empty: caret visible
	RunFrameFn(view)
	focusedRect := st.textRect
	textFrame("ab", view)
	RunFrameFn(view)
	textRect := st.textRect
	keyFrame(KeyEscape, 0, view) // clears + blurs: hint state
	RunFrameFn(view)
	hintRect := st.textRect
	if focusedRect.Origin[1] != textRect.Origin[1] || textRect.Origin[1] != hintRect.Origin[1] {
		t.Errorf("text row Y = %v/%v/%v across states, want identical (no jitter on focus)",
			focusedRect.Origin[1], textRect.Origin[1], hintRect.Origin[1])
	}
	if focusedRect.Size[1] != textRect.Size[1] || textRect.Size[1] != hintRect.Size[1] {
		t.Errorf("text row height = %v/%v/%v across states, want identical",
			focusedRect.Size[1], textRect.Size[1], hintRect.Size[1])
	}
}
