//go:build linux

package waylandbackend

import (
	"time"

	wos "go.hasen.dev/shirei/internal/wayland/os"
	"go.hasen.dev/shirei/internal/wayland/wl"
	"go.hasen.dev/shirei/internal/wayland/wlclient"
	xkb "go.hasen.dev/shirei/internal/wayland/xkbcommon"

	g "go.hasen.dev/generic"
	"go.hasen.dev/shirei"
	"go.hasen.dev/shirei/internal/qwerty"
)

// Keyboard input. The compositor hands us an xkb keymap over an fd; we compile it
// with libxkbcommon (via neurlang's purego binding — no cgo) and keep an xkb state
// updated from wl_keyboard.modifiers. Per key we get the keysym (an X11 keysym
// value, same as the X11 backend uses) for shortcut mapping, and the UTF-32
// codepoint for typed text. Sampled into the shirei globals (sample, not queue).

var (
	keyboard   *wl.Keyboard
	xkbContext *xkb.Context
	xkbKeymap  *xkb.Keymap
	xkbState   *xkb.State

	// PATCHED by optiscaler-manager (v0.10): client-side key repeat — reapply after `go mod vendor` (see docs/vendor-patches.md).
	repeatKey      uint32        // xkb keycode armed for repeat (0 = none)
	repeatKeysym   uint32        // keysym captured at arm time
	repeatDelay    time.Duration // delay to first repeat
	repeatInterval time.Duration // 0 when the compositor asked for no repeats
	repeatNext     time.Time     // next synthetic-press due time
)

// defaultRepeatDelay/defaultRepeatInterval: fallbacks when the compositor never sends repeat_info. Matches the spec the UI documents: 300 ms to first repeat, then 20 Hz. PATCHED (v0.10).
const (
	defaultRepeatDelay    = 300 * time.Millisecond
	defaultRepeatInterval = 50 * time.Millisecond
)

// Bits of the serialized xkb modifier mask (standard keymaps use the X11
// convention, matching x11backend).
const (
	wlModShift = 1 << 0
	wlModCtrl  = 1 << 2
	wlModAlt   = 1 << 3
	wlModSuper = 1 << 6
)

// ensureKeyboard attaches a wl_keyboard listener once the seat advertises one.
func ensureKeyboard() {
	if keyboard != nil {
		return
	}
	if xkbContext == nil {
		xkbContext = xkb.ContextNew(xkb.ContextNoFlags)
	}
	kb, err := seat.GetKeyboard()
	if err != nil {
		return
	}
	keyboard = kb
	wlclient.KeyboardAddListener(keyboard, h)
}

// HandleKeyboardKeymap compiles the keymap the compositor sends over an fd.
func (*handler) HandleKeyboardKeymap(ev wl.KeyboardKeymapEvent) {
	if ev.FdError != nil {
		return
	}
	defer wos.Close(int(ev.Fd))
	if ev.Format != xkb.KeymapFormatTextV1 || xkbContext == nil {
		return
	}
	data, err := wos.Mmap(int(ev.Fd), 0, int(ev.Size), wos.ProtRead, wos.MapPrivate)
	if err != nil {
		return
	}
	defer wos.Munmap(data)

	km := xkbContext.KeymapNewFromString(data, xkb.KeymapFormatTextV1, 0)
	if km == nil {
		perfLog("[wl] keymap compile failed")
		return
	}
	st := km.StateNew()
	if st == nil {
		return
	}
	xkbKeymap, xkbState = km, st
}

// HandleKeyboardModifiers feeds the serialized modifier state into xkb (so text
// reflects shift/caps/layout) and into the shirei modifier flags.
func (*handler) HandleKeyboardModifiers(ev wl.KeyboardModifiersEvent) {
	if xkbState == nil {
		return
	}
	xkbState.UpdateMask(ev.ModsDepressed, ev.ModsLatched, ev.ModsLocked, 0, 0, ev.Group)
	updateModifiers(ev.ModsDepressed | ev.ModsLatched)
	dirty = true
	wlDebug("modifiers: depressed=%#x latched=%#x locked=%#x -> shirei mods=%04b",
		ev.ModsDepressed, ev.ModsLatched, ev.ModsLocked, shirei.GetInputState().Modifiers)
}

func (*handler) HandleKeyboardKey(ev wl.KeyboardKeyEvent) {
	lastSerial = ev.Serial // for set_selection (copy via Ctrl+C)
	if xkbState == nil {
		return
	}
	code := ev.Key + 8 // evdev keycode -> xkb keycode
	down := ev.State != wl.KeyboardKeyStateReleased
	onKey(code, xkbState.KeyGetOneSym(code), down)
	dirty = true
	wlDebug("key: evdev=%d down=%v (mods now %04b)", ev.Key, down, shirei.GetInputState().Modifiers)
}

func (*handler) HandleKeyboardEnter(wl.KeyboardEnterEvent) { wlDebug("keyboard enter") }

// HandleKeyboardLeave: focus lost — drop held keys so none stick. Composition
// is also cleared by text-input leave (which tracks the same focus); clear
// here too so a compositor that omits text-input leave cannot leave a stale
// underline.
func (*handler) HandleKeyboardLeave(wl.KeyboardLeaveEvent) {
	shirei.GetInputState().DownKeys = shirei.GetInputState().DownKeys[:0]
	shirei.GetInputState().Modifiers = 0
	cancelRepeat() // PATCHED (v0.10): focus lost — stop repeating
	clearComposition()
	dirty = true
	wlDebug("keyboard leave")
}

// HandleKeyboardRepeatInfo stores the compositor-supplied rate/delay so armRepeat/pumpRepeat can synthesize presses. Wayland puts key repeat on the client. PATCHED by optiscaler-manager (v0.10): previously a no-op.
func (*handler) HandleKeyboardRepeatInfo(ev wl.KeyboardRepeatInfoEvent) {
	repeatDelay = time.Duration(ev.Delay) * time.Millisecond
	if ev.Rate > 0 {
		repeatInterval = time.Second / time.Duration(ev.Rate)
	} else {
		repeatInterval = 0 // Rate 0 means "don't repeat" per the protocol.
		repeatKey = 0
	}
}

// armRepeat begins synthesizing presses for code/keysym if the keymap says the key repeats. Pressing a different key implicitly cancels the previous arm. PATCHED (v0.10).
func armRepeat(code, keysym uint32) {
	if xkbKeymap == nil || !xkbKeymap.KeyRepeats(code) {
		repeatKey = 0
		return
	}
	if repeatDelay <= 0 {
		repeatDelay = defaultRepeatDelay
	}
	if repeatInterval == 0 && repeatDelay > 0 {
		repeatInterval = defaultRepeatInterval
	}
	if repeatInterval <= 0 {
		repeatKey = 0
		return
	}
	repeatKey = code
	repeatKeysym = keysym
	repeatNext = time.Now().Add(repeatDelay)
}

// cancelRepeat stops synthesizing presses. PATCHED (v0.10).
func cancelRepeat() { repeatKey = 0 }

// pumpRepeat delivers one synthetic press if a held key's repeat is due. Called from the main loop after each dispatch so the cadence tracks real time without a goroutine. PATCHED (v0.10).
func pumpRepeat() {
	if repeatKey == 0 || repeatInterval <= 0 {
		return
	}
	now := time.Now()
	if now.Before(repeatNext) {
		return
	}
	onKey(repeatKey, repeatKeysym, true)
	repeatNext = now.Add(repeatInterval)
	dirty = true
}

// repeatTimeout caps the dispatch wait so a pending repeat wakes the loop in time. PATCHED (v0.10).
func repeatTimeout(max time.Duration) time.Duration {
	if repeatKey == 0 || repeatInterval <= 0 {
		return max
	}
	wait := time.Until(repeatNext)
	if wait < 0 {
		return 0
	}
	if wait < max {
		return wait
	}
	return max
}

// onKey maps a key event to a shirei key code and, for printable presses, the
// typed text. Mirrors x11backend.onKey. The writing block resolves by evdev
// position — KeyW is the physical key at the US-QWERTY W position no matter
// the layout; the layout still drives the typed text below. Other keys
// resolve by keysym.
//
// While an IME composition is active, editing/navigation keys and typed text
// belong to the IME (Cocoa B1 hasMarkedText / Win32 VK_PROCESSKEY). text-input-v3
// has no per-key "consumed" flag, so we gate on non-empty Composition
// (textInputConsumesKeys). Committed text arrives via commit_string and is
// merged into pendingText on done.
//
// Do NOT suppress xkb→utf32 merely because text-input-v3 is enabled. GNOME
// Mutter (and similar) only sends commit_string for IME-routed text; ordinary
// latin/digit keys arrive solely on wl_keyboard.key with no commit_string.
// Gating on textInputEnabled left TextInput fields read-only without an IME
// (go-shirei#15). While composing, the early return above already skips xkb
// text — that is enough to avoid double-insert with preedit/commit_string.
func onKey(code, keysym uint32, down bool) {
	kc := qwerty.FromScan(uint16(code - 8)) // xkb keycode -> evdev
	if kc == shirei.KeyCodeNone {
		kc = mapKeysym(keysym)
	}
	composing := textInputConsumesKeys()
	if kc != shirei.KeyCodeNone {
		if down {
			if !composing {
				shirei.GetFrameInput().Key = kc
			}
			g.SliceAddUniq(&shirei.GetInputState().DownKeys, kc)
			armRepeat(code, keysym) // PATCHED (v0.10): begin client-side repeat
		} else {
			g.SliceRemove(&shirei.GetInputState().DownKeys, kc)
			if code == repeatKey {
				cancelRepeat() // PATCHED (v0.10): released the armed key
			}
		}
	}
	if !down || composing {
		return
	}
	// Suppress text for shortcut combos and control characters (delivered as Key).
	if shirei.GetInputState().Modifiers&(shirei.ModCtrl|shirei.ModCmd|shirei.ModAlt) != 0 {
		return
	}
	// Plain typing: xkb utf32. Accumulate so multi-key frames keep all
	// characters (assign would drop earlier ones, same class of bug as Win32 W0).
	if r := rune(xkbState.KeyGetUtf32(code)); r >= 0x20 && r != 0x7f {
		appendPendingText(string(r))
	}
}

func updateModifiers(mask uint32) {
	var m shirei.Modifiers
	if mask&wlModShift != 0 {
		m |= shirei.ModShift
	}
	if mask&wlModCtrl != 0 {
		m |= shirei.ModCtrl
	}
	if mask&wlModAlt != 0 {
		m |= shirei.ModAlt
	}
	if mask&wlModSuper != 0 {
		m |= shirei.ModSuper
	}
	shirei.GetInputState().Modifiers = m

	syncModKey(m, shirei.ModShift, shirei.KeyShift)
	syncModKey(m, shirei.ModCtrl, shirei.KeyCtrl)
	syncModKey(m, shirei.ModAlt, shirei.KeyAlt)
	syncModKey(m, shirei.ModSuper, shirei.KeySuper)
}

func syncModKey(m, bit shirei.Modifiers, k shirei.KeyCode) {
	if m&bit != 0 {
		g.SliceAddUniq(&shirei.GetInputState().DownKeys, k)
	} else {
		g.SliceRemove(&shirei.GetInputState().DownKeys, k)
	}
}

// X11 keysym constants (xkb uses the same values). Mirrors x11backend; a shared
// keysym->KeyCode helper is a candidate cleanup.
const (
	xkBackSpace  = 0xff08
	xkTab        = 0xff09
	xkISOLeftTab = 0xfe20 // PATCHED by optiscaler-manager (v0.9): Shift+Tab keysym — reapply after `go mod vendor` (see docs/vendor-patches.md)
	xkReturn     = 0xff0d
	xkEscape     = 0xff1b
	xkHome       = 0xff50
	xkLeft       = 0xff51
	xkUp         = 0xff52
	xkRight      = 0xff53
	xkDown       = 0xff54
	xkPrior      = 0xff55 // Page Up
	xkNext       = 0xff56 // Page Down
	xkEnd        = 0xff57
	xkDelete     = 0xffff
	xkSpace      = 0x0020
	xkF1         = 0xffbe
	xkF12        = 0xffc9
)

func mapKeysym(ks uint32) shirei.KeyCode {
	switch ks {
	case xkLeft:
		return shirei.KeyLeft
	case xkRight:
		return shirei.KeyRight
	case xkUp:
		return shirei.KeyUp
	case xkDown:
		return shirei.KeyDown
	case xkReturn:
		return shirei.KeyEnter
	case xkEscape:
		return shirei.KeyEscape
	case xkBackSpace:
		return shirei.KeyDeleteBackward
	case xkDelete:
		return shirei.KeyDeleteForward
	case xkHome:
		return shirei.KeyHome
	case xkEnd:
		return shirei.KeyEnd
	case xkb.KeyInsert:
		return shirei.KeyInsert
	case xkPrior:
		return shirei.KeyPageUp
	case xkNext:
		return shirei.KeyPageDown
	case xkTab:
		return shirei.KeyTab
	case xkISOLeftTab: // PATCHED by optiscaler-manager (v0.9): Shift+Tab -> KeyTab so the toolkit reverse-cycles focus
		return shirei.KeyTab
	case xkSpace:
		return shirei.KeySpace
	}
	if ks >= xkF1 && ks <= xkF12 {
		return shirei.KeyF1 + shirei.KeyCode(ks-xkF1)
	}
	if ks >= 'a' && ks <= 'z' {
		return shirei.KeyA + shirei.KeyCode(ks-'a')
	}
	if ks >= 'A' && ks <= 'Z' {
		return shirei.KeyA + shirei.KeyCode(ks-'A')
	}
	if ks >= '0' && ks <= '9' {
		return shirei.Key0 + shirei.KeyCode(ks-'0')
	}
	return shirei.KeyCodeNone
}
