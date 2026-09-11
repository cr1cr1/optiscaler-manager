---
type: reference
---

# Vendor patches

Local patches applied on top of vendored dependencies (`vendor/`). Every
patch carries a marker comment so it can be found and reapplied, and is
guarded by a test so a silent revert (e.g. after `go mod vendor`) fails CI.

Patches were reapplied verbatim onto shirei v0.6.7 (2026-08-28) and
reapplied again onto shirei v0.6.10 (2026-09-11, after the `go mod vendor`
in the dependency upgrade wiped them). The v0.6.10 reapplication spliced
each hunk at its new location and adapted two hunks to upstream changes:
v0.12 and v0.15 are **superseded** (v0.6.10 implements both natively) and
are no longer reapplied. Two other adaptations: the Wayland key-repeat
loop wiring moved into v0.6.10's restructured dispatch loop (same
`repeatTimeout(framePoll)` / `pumpRepeat()` calls), and the keyboard patch
file is the old patched source with v0.6.10's `WindowFocused`
enter/leave bodies substituted in; the Win32 repeat patch moved its
`armRepeat`/`cancelRepeat` hooks into v0.6.10's `onKey` and `pumpRepeat`
into the `wmTimer` case. The upgrade also removed shirei's opt-in
`CycleFocusOnTab` (Tab cycling became automatic in the frame loop) and
made `widgets.ButtonExt`/`ToggleSwitchExt` faces self-focusing with
press→release key activation, which `internal/gui` adapted to (see
`internal/gui/widgets.go`).

## shirei: dark Wayland CSD titlebar (v0.5)

- **File**: `vendor/go.hasen.dev/shirei/waylandbackend/waylanddecor_linux.go`
- **Marker**: `// PATCHED by optiscaler-manager (v0.5)`
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent`)

**What.** shirei v0.6.6's Wayland backend draws its own client-side
decorations (CSD) with a hardcoded light titlebar. The patch retints the
titlebar and its controls to the app's dark palette.

**Why.** The whole GUI is dark-themed; a light titlebar on Wayland looks
broken next to it. The fix lives in the vendor tree because shirei has no
theming hook for its decorations and the pinned release cannot be changed
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

**What.** shirei v0.6.6's Wayland backend resolves keysyms with
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

**What.** shirei v0.6.6's Wayland backend explicitly left
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

## shirei: Wayland resize redraw (v0.12) — SUPERSEDED

**Status.** No longer reapplied: shirei v0.6.10 fixes this natively —
`HandleSurfaceConfigure` sets `dirty = true` whenever a resize is acked
("an interactive resize holds a pointer grab, so no input event sets
`dirty`"). The guard test now asserts the upstream mechanism instead of the
old marker, so an accidental shirei downgrade fails loudly. History below
is kept for the record.

**What.** shirei v0.6.6's `HandleToplevelConfigure` updated the window's
logical size on resize but never set `dirty = true`, so the Wayland main
loop's `if dirty && frameCb == nil { drawFrame() }` never fired — the app
didn't repaint on resize until the next unrelated input event arrived.

## shirei: disable layout animations (v0.13)

- **File**: `vendor/go.hasen.dev/shirei/shirei.go` (the `rate` line in `resolveOrigins`).
- **Marker**: `// PATCHED by optiscaler-manager (v0.13)`.
- **Guard**: `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent` checks for the v0.13 marker in `shirei.go`).

**What.** `resolveOrigins` animates every container's size, origin,
padding, and corners toward its new target at `rate = min(1, ui.timeDelta*20)`.
The patch forces `rate := float32(1)` so every container snaps to its target
immediately — no smooth transitions on resize, panel open/close, view switch,
or hover lift. The user explicitly requested no animations; the 5-7×
reduction in repaint frames during any layout change is a bonus.

(v0.6.6 reworked animation into a per-channel `Animations` bitfield with
`NoAnimate`/`AnimateOnly` attrs. Our code already uses `NoAnimate` on
modals/overlays; the global `rate=1` here is the broader kill switch. With
it, the per-container flags are moot for layout channels — revisit if
selective animations are wanted later.)

**Why.** Each layout change now produces exactly 1 repaint frame instead
of 5-7 (the animation chain was the dominant cause of resize sluggishness
on Wayland, 4-16 fps during drag).

**Scope.** All backends (shirei core). Applied once in `resolveOrigins`.

**Reapplying after `go mod vendor`.** Replace `var rate = min(1, ui.timeDelta*20)`
with `rate := float32(1)` (plus the marker comment); `TestVendorCSDPatchPresent`
fails while the marker is missing.

## shirei: cover-art stretch-to-fill (v0.14)

- **Files**: `vendor/go.hasen.dev/shirei/softrender.go` (`ImageScale` block: stretch both axes to the surface rect), `vendor/go.hasen.dev/shirei/images.go` (new `ImageFill` function).
- **Markers**: `// PATCHED by optiscaler-manager (v0.14)`.
- **Guard**: `internal/gui/csd_test.go` (checks for the v0.14 marker in `softrender.go` and `func ImageFill(` in `images.go`).

**What.** Upstream `ImageScale` fits an image to the surface *height* only,
leaving a horizontal gap when the image's aspect ratio doesn't match the
container's (e.g. a 460×900 cover in a 260×390 card slot). The patch
stretches to fill the surface rect exactly on both axes, and adds `ImageFill`
(like `Image` but ignores aspect ratio) for grid-view cover thumbnails where
a gap is worse than minor distortion (imperceptible for near-2:3 art).

**Reapplying after `go mod vendor`.** Re-apply the stretch in `softrender.go`
(`dwl, dhl = s.Rect.Size[0], s.Rect.Size[1]`) and re-add `ImageFill` in
`images.go`; the guard fails while either is missing.

## shirei: Wayland skip-unchanged-frames (v0.15) — SUPERSEDED

**Status.** No longer reapplied: shirei v0.6.10 implements the same
early-return natively in `drawFrame` (`out.SurfacesHash == lastPresentedHash`
plus size equality and no pending ack). The guard test asserts the upstream
mechanism. History below is kept for the record.

**What.** The Wayland backend always rasterized + `Attach` + `Damage` +
`Commit`-ted every frame, even when nothing changed. v0.6.6 made
`FrameHasChanges` hash-based (precise "did the rendered content change"),
but the backend never consulted it. The patch skipped the expensive paint
when `!out.FrameHasChanges && haveFrame` — `RunFrameFn` still ran (input,
app state, hover, clipboard/IME outputs were processed); only the software
raster + compositor recomposite was skipped.

## shirei: headless identity-tree reset (v0.16)

- **File**: `vendor/go.hasen.dev/shirei/renderpng.go` (`ResetInputSession`: reset `ui.identRoot` + build buffers).
- **Marker**: `// PATCHED by optiscaler-manager (v0.16)`.
- **Guard**: `internal/gui/csd_test.go` (checks for the v0.16 marker in `renderpng.go`).

**What.** `ResetInputSession` is the headless/test reset (called by
`RenderToImage` and the GUI test helper). It reset input and the focus graph
but not the identity tree, so `Use`-hook-backed widget state — virtual-list
scroll and height caches — leaked across back-to-back tests. v0.6.6's
stricter virtual list exposed this as order-dependent failures. The patch
resets `ui.identRoot` (and `currentIdent`, the surfaces/popups/pending-commands
build buffers, `surfaceHash`, and the sweep counter) to a fresh state, giving
true per-run isolation while preserving `Host` (the caller sets `WindowSize`).

**Scope.** Headless only — every `ResetInputSession` caller is headless
(`RenderToImage`, the GUI test helper); live backends never call it.

**Reapplying after `go mod vendor`.** Re-add the identity-tree + build-buffer
reset block at the end of `ResetInputSession`.

## shirei: mouse cursor shape (v0.17)

- **Files**: `vendor/go.hasen.dev/shirei/shirei.go` (cursor constants,
  `MouseCursorShape` var, the hover-chain rule in `RunFrameFn`, the
  `TextEntry`/`PointerHand` attrs on `AttrSet`), `attrs.go` (the two
  setters), `waylandbackend/waylandcursor_linux.go` (shape-aware
  `applyCursor` + themed hand loading + `applyPointerCursor` tracking),
  `waylandbackend/waylandbackend_linux.go` (the post-frame hook before the
  unchanged-frame early return).
- **Marker**: `// PATCHED by optiscaler-manager (v0.17)` in all four files.
- **Guard**: `internal/gui/csd_test.go` (checks all four markers) and
  `internal/gui/cursor_test.go` (behavior: the picked shape per hover
  target).

**What.** Every clickable surface showed the same arrow cursor: shirei had
no cursor-shape concept. The patch adds `CursorShapeDefault` (1) and
`CursorShapePointer` (4) — the `wp_cursor_shape_device_v1.shape` values —
plus an exported `MouseCursorShape` var that `RunFrameFn`'s hover-detection
block picks from the hover chain, innermost first: a `TextEntry` container
keeps the arrow (the caret signals editability); the first `Focusable` or
`PointerHand` container asks for the pointing hand; empty space stays the
arrow.

The wayland backend applies the picked shape after every `RunFrameFn` —
before the unchanged-frame early return, so hover moves change the cursor
without a repaint — tracking `(pointerSerial, shape)` and re-issuing only
on change. Tier mapping: wp_cursor_shape gets the protocol shape directly
(`pointer` = 4); the themed-xcursor tier loads the theme's hand
(`hand2`, `hand`, `pointing_hand`, `pointer`, in order, then the theme's
default); the drawn-bitmap fallback tier keeps the arrow (theme-less
systems only — a hand bitmap is not worth it).

**Scope.** Wayland only. X11 and Win32 keep their static cursors; shirei
has no per-frame cursor hook for them. The app opts in with
`PointerHand` where a click affordance has no `Focusable` of its own
(sidebar items, view-switch segments, dropdown rows); focusable containers
(buttons, cards, dropdown triggers, DLSS controls) get the hand via
`Focusable`; text inputs set `TextEntry`.

**Reapplying after `go mod vendor`.** Re-add: the const/var block before
`IdIsHovered`; the cursor rule inside the hover-chain walk (after the
`ui.hoverList` append, guarded by a `cursorDecided` bool); the two
`AttrSet` fields after `Focusable`; the setters after `Focusable` in
`attrs.go`; `shapePointer`/`handCursorNames`, the shape-keyed
`themedImage`/`themedTried` maps, the shape-aware `themedCursorImage`,
`applyPointerCursor` and the shape-aware `applyCursor` in
`waylandcursor_linux.go` (plus the `shirei` import); the post-frame
`applyPointerCursor()` call in `drawFrame` before the early return.
`TestVendorCSDPatchPresent` and the cursor tests fail while any part is
missing.
