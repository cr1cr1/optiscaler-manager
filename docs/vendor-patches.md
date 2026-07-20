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
