---
type: reference
---

# Vendor patches

Local patches applied on top of vendored dependencies (`vendor/`). Every
patch carries a marker comment so it can be found and reapplied, and is
guarded by a test so a silent revert (e.g. after `go mod vendor`) fails CI.

## shirei: dark Wayland CSD titlebar (v0.5)

- **File**: `vendor/go.hasen.dev/shirei/waylandbackend/waylanddecor_linux.go`
- **Marker**: `// PATCHED by optiscaler-manager (v0.5)`
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent`)

**What.** shirei v0.5.2's Wayland backend draws its own client-side
decorations (CSD) with a hardcoded light titlebar. The patch retints the
titlebar and its controls to the app's dark palette.

**Why.** The whole GUI is dark-themed; a light titlebar on Wayland looks
broken next to it. The fix lives in the vendor tree because shirei has no
theming hook for its decorations and the pinned v0.5.2 cannot be changed
upstream on our schedule.

**Scope.** Wayland only. On X11 the window manager draws the decorations,
so nothing here applies.

**Reapplying after `go mod vendor`.** `go mod vendor` rewrites the vendor
tree from the module cache and drops the patch. When that happens:

1. Reapply the dark-palette change to
   `vendor/go.hasen.dev/shirei/waylandbackend/waylanddecor_linux.go`.
2. Keep the marker comment on the patched line(s).
3. Run `go test ./internal/gui/` — `TestVendorCSDPatchPresent` fails while
   the marker is missing.

**Upgrade path.** Remove the patch when upgrading to a shirei release that
themes its CSD titlebar natively; update this page at the same time.

## shirei: CSD disabled + scroll speedup (v0.8)

- **Files**: `vendor/go.hasen.dev/shirei/waylandbackend/waylanddecor_linux.go` (`csdEnabled = false`), `vendor/go.hasen.dev/shirei/shirei.go` (`ScrollOnInput` ×2/×3 wheel multiplier).
- **Markers**: `// PATCHED by optiscaler-manager (v0.8)`
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent`).

**What.** Two user-requested changes: the client-side Wayland titlebar is
turned off entirely (`csdEnabled = false` — the OS window manager keeps its
default decorations where the compositor provides them, none where it does
not), and wheel scrolling is sped up 2× horizontally / 3× vertically in
`ScrollOnInput` (shirei's raw 1:1 deltas felt slow on Linux; Win32's
30px/notch becomes 90px, roughly three text lines, matching OS conventions).

**Why.** User-facing layout and feel requests; shirei has no theming or
speed hooks for either behavior.

**Reapplying after `go mod vendor`.** Reapply both edits with the marker
comments; `TestVendorCSDPatchPresent` fails while either marker is missing.
The earlier dark-CSD retint (v0.5) is now inert while CSD is disabled; keep
the v0.5 patch text in place so re-enabling is one flag away.

## shirei: Wayland Shift+Tab reverse focus cycling (v0.9)

- **File**: `vendor/go.hasen.dev/shirei/waylandbackend/waylandkeyboard_linux.go` (`xkISOLeftTab` const + `mapKeysym` case).
- **Marker**: `// PATCHED by optiscaler-manager (v0.9)` (trailing on both added lines; guard also looks for `xkISOLeftTab`).
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent`); behavior covered by `internal/gui/widgets_test.go` (`TestFocusableButtonTabCyclesAndEnterActivates` — Shift+Tab reverse-cycles).

**What.** shirei v0.5.2's Wayland backend resolves keysyms with
`xkbState.KeyGetOneSym`; with Shift held, Tab yields `ISO_Left_Tab`
(0xFE20), which `mapKeysym` did not know, so `FrameInput.Key` was never
set and the keypress vanished. The patch adds the `xkISOLeftTab = 0xfe20`
const and a `mapKeysym` case mapping it to `shirei.KeyTab` — the toolkit's
`_cycleFocusOnTab` already reads `ModShift` and reverse-cycles, so no
toolkit change is needed. X11, Win32, and Cocoa deliver Shift+Tab
correctly unpatched.

**Why.** Keyboard-only users could not reverse-cycle focus on Wayland;
every other backend handles it.

**Reapplying after `go mod vendor`.** Re-add the const next to `xkTab`
and the case next to `case xkTab:`, each with the trailing marker comment;
`TestVendorCSDPatchPresent` fails while the marker is missing.

## shirei: Wayland client-side key repeat (v0.10)

- **Files**: `vendor/go.hasen.dev/shirei/waylandbackend/waylandkeyboard_linux.go` (`repeatKey`/`repeatDelay`/`repeatInterval`/`repeatNext` vars, `defaultRepeatDelay`/`defaultRepeatInterval` consts, `HandleKeyboardRepeatInfo` body, `armRepeat`/`cancelRepeat`/`pumpRepeat`/`repeatTimeout` helpers, two call sites in `onKey`); `vendor/go.hasen.dev/shirei/waylandbackend/waylandbackend_linux.go` (`pumpRepeat()` + `repeatTimeout(framePoll)` in the dispatch loop).
- **Markers**: `// PATCHED by optiscaler-manager (v0.10)` (header block on the vars; trailing on the two `onKey` callsites, the `HandleKeyboardRepeatInfo` body, and the two `waylandbackend_linux.go` loop edits).
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent` checks for `HandleKeyboardRepeatInfo` + `pumpRepeat` in the keyboard file and `pumpRepeat()` + `repeatTimeout(framePoll)` in the loop file).

**What.** shirei v0.5.2's Wayland backend explicitly left
`HandleKeyboardRepeatInfo` as a no-op ("client-side key repeat isn't
implemented yet"), so the compositor-supplied repeat rate/delay was
discarded and held keys fired exactly once before the app went idle. The
patch implements client-side repeat the way the Wayland protocol intends:

1. `HandleKeyboardRepeatInfo` stores `Rate` (chars/sec) and `Delay` (ms)
   from `wl_keyboard.repeat_info`. If the compositor never sends one,
   `armRepeat` falls back to 300 ms delay / 20 Hz interval — the rate
   the spec documents for this app.
2. `onKey` calls `armRepeat` on every press. `xkbKeymap.KeyRepeats(code)`
   gates on the keymap's per-key repeat flag (modifiers don't repeat;
   arrows / Tab / Backspace / etc. do), and arming a new key cancels the
   previous one (matches OS repeat — `FrameInput.Key` is single-slot, so
   stale repeats would shadow the new press).
3. `pumpRepeat` runs from the main dispatch loop after every
   `DisplayDispatchTimeout`. If a repeat is due it synthesizes a press
   via `onKey(...,true)` and bumps `dirty`, so the next `drawFrame` runs
   and the synthesized `FrameInput.Key` actually reaches the app.
4. `repeatTimeout` caps the dispatch wait so the loop wakes precisely
   when the next repeat is due, instead of waiting up to `framePoll`
   (16 ms) past it.

**Why.** Without this, holding arrow/Tab/Backspace anywhere in the GUI
fired once and then nothing — visible in cards, list view, text fields,
dropdowns. X11/Win32/Cocoa already deliver repeats natively (X server
auto-repeat / WM_KEYDOWN auto-repeat / NSEvent isARepeat); Wayland was
the only broken backend.

**Thread safety.** All repeat state lives on the Wayland main goroutine:
event handlers run inside `DisplayDispatchTimeout`, `pumpRepeat` runs
right after it, and `drawFrame` runs in the same loop iteration. No
background goroutine, no mutex needed.

**Reapplying after `go mod vendor`.** Re-add the `time` import if needed,
the `repeatKey`/`repeatKeysym`/`repeatDelay`/`repeatInterval`/`repeatNext`
vars (with the header marker comment), the `defaultRepeatDelay` /
`defaultRepeatInterval` consts, the body of `HandleKeyboardRepeatInfo`,
the four helper functions, the two call sites in `onKey` (each with the
trailing marker comment), and the two edits in the dispatch loop
(`repeatTimeout(framePoll)` and `pumpRepeat()`); `TestVendorCSDPatchPresent`
fails while any marker is missing.

## shirei: Win32 client-side key repeat (v0.11)

- **File**: `vendor/go.hasen.dev/shirei/win32backend/win32backend_windows.go` (`repeatWparam`/`repeatLparam`/`repeatArmed`/`repeatDelay`/`repeatInterval`/`repeatNext` vars, `defaultRepeatDelay`/`defaultRepeatInterval` consts, `armRepeat`/`cancelRepeat`/`pumpRepeat` helpers, three call sites: `onKey` press/release and `wmKillfocus` and `wmTimer`).
- **Markers**: `// PATCHED by optiscaler-manager (v0.11)` (header block on the vars and on the consts; trailing on every call site and on each helper docstring).
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent` checks for `armRepeat` + `pumpRepeat` + the marker in `win32backend_windows.go`).

**What.** WM_KEYDOWN auto-repeat is *supposed* to arrive natively on
Windows (and on most setups it does — the agent's static analysis of the
win32backend confirmed there is no `KF_REPEAT` filter or message-loop
suppression anywhere in the code path). Yet on some setups — RDP sessions,
VMs with passthrough keyboards, FilterKeys / accessibility configurations —
repeats do not arrive, and the symptom matches the v0.10 Wayland bug
exactly: first press fires once, then nothing.

This patch adds the same defensive client-side repeat pattern as v0.10:

1. `onKey` calls `armRepeat(wparam, lparam)` on every press and
   `cancelRepeat()` on every release. Arming captures the wparam/lparam so
   `pumpRepeat` can re-fire the same key via `onKey`.
2. `pumpRepeat` is called from `wmTimer` (the existing 16 ms animation
   tick). If a repeat is due it synthesizes a press via `onKey(...,true)`
   and `noteInput()`s to drive the next frame.
3. **No-op when native works.** Each native WM_KEYDOWN auto-repeat calls
   `armRepeat`, which resets `repeatNext = now + repeatDelay` (300 ms). So
   if native repeats arrive at any rate faster than the delay, pumpRepeat
   never fires. Native rate wins. The patch only takes over when native
   repeats stop arriving.

**Why.** Manual testing on Windows showed held keys firing exactly once.
Static analysis found no code-level filter, but the empirical bug was
real. The defensive patch is a no-op on healthy systems and a guaranteed
fix on the environments where native repeat is absent.

**Scope.** Windows only. Defaults to 300 ms / 20 Hz (matches the
specification; no `SystemParametersInfoW` OS-rate query yet — that's a
candidate follow-up). Win32 native auto-repeat, when present, always wins.

**Thread safety.** All repeat state lives on the UI thread: `wndProc`
runs there, `wmTimer` is dispatched there, `messageLoop` blocks there.
No background goroutine, no mutex.

**Reapplying after `go mod vendor`.** Re-add the `repeatWparam` /
`repeatLparam` / `repeatArmed` / `repeatDelay` / `repeatInterval` /
`repeatNext` vars and the two `defaultRepeat*` consts (each with the
header marker comment), the three helper functions
(`armRepeat`/`cancelRepeat`/`pumpRepeat`), and the four call sites
(`onKey` press arm, `onKey` release cancel, `wmKillfocus` cancel,
`wmTimer` pump — each with the trailing marker comment);
`TestVendorCSDPatchPresent` fails while any marker is missing.

## shirei: Wayland resize redraw (v0.12)

- **File**: `vendor/go.hasen.dev/shirei/waylandbackend/waylandbackend_linux.go` (one line in `HandleToplevelConfigure`).
- **Marker**: `// PATCHED by optiscaler-manager (v0.12)` (trailing on the `dirty = true` line).
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent` checks for the v0.12 marker in `waylandbackend_linux.go`).

**What.** shirei v0.5.2's `HandleToplevelConfigure` updated the window's
logical size on resize but never set `dirty = true`, so the Wayland main
loop's `if dirty && frameCb == nil { drawFrame() }` never fired — the app
didn't repaint on resize until the next unrelated input event arrived.

**Why.** Resize felt broken on Wayland: the window changed size but the
content stayed stale, then snapped forward on the next mouse motion.

**Scope.** Wayland only. X11 (`ConfigureNotify → dirty`, `x11input.go:70`)
and Win32 (`wmSize → noteInput`, `win32backend_windows.go:225`) already
flag dirty on resize.

**Reapplying after `go mod vendor`.** Re-add `dirty = true` with the
trailing marker comment inside the `HandleToplevelConfigure` size-change
block; `TestVendorCSDPatchPresent` fails while the marker is missing.

## shirei: snap layout animations during resize (v0.13)

- **File**: `vendor/go.hasen.dev/shirei/shirei.go` (`prevWindowSize`/`windowResizedNow` vars + detection in the frame pass + rate override in `resolveOrigins`).
- **Marker**: `// PATCHED by optiscaler-manager (v0.13)`.
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent` checks for `windowResizedNow` in `shirei.go`).

**What.** `resolveOrigins` (shirei.go:~1095) animates every container's
size, origin, padding, and corners toward their new target at `rate =
min(1, timeDelta*20)`. A window resize moves every container, so after
each resize tick the UI animates for ~5-7 extra frames; each animation
frame changes surfaces → `FrameHasChanges` → `NextFrameRequested` →
backend repaints again. This animation chain was the dominant cause of
resize sluggishness on Wayland (4-16 fps during drag).

The patch adds a per-frame `windowResizedNow` flag (comparing `WindowSize`
to the previous frame's) and sets `rate = 1` (snap) when it's true. The
visual difference is imperceptible — during a drag the user is focused
on the window edge, not content smoothness. Non-resize animations (panel
open/close, view switch, hover lift) are unaffected because they don't
change `WindowSize`.

**Why.** Each resize tick now produces exactly 1 repaint frame instead
of 5-7, a 5-7× reduction in resize-time frame production.

**Scope.** All backends (shirei core). The flag is computed once per
`RunFrameFn` pass and read in `resolveOrigins`.

**Reapplying after `go mod vendor`.** Re-add the `prevWindowSize`/
`windowResizedNow` vars (with marker), the detection lines after
`timeDelta = ...` in the frame pass, and the rate override in the
animation block of `resolveOrigins`; `TestVendorCSDPatchPresent` fails
while the marker is missing.
