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
