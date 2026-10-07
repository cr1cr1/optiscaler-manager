---
title: Discover games in non-standard directory layouts, and add manual title + manual poster overrides
description: Eight library rows sit on placeholder covers for three general reasons — UE packaged-build staging dirs (IntermediateBuildDRM/WindowsNoEditor) become the row instead of the real game root, exes deeper than maxExeDepth=4 are invisible, and helper binaries like elevate.exe win when every real exe is skip-token'd — plus several correct-but-unmatchable titles (GOG edition strings, PE junk like "ControlLauncher", non-Steam games). The scanner now treats UE staging dirs as transparent engine folders, searches one level deeper, and skips elevate; and both frontends gain a per-game "set title" (UI for the existing title_overrides) and "upload poster" (new cover_overrides, sticky user art that beats the whole fetch chain).
---

# 028 — Non-standard directory heuristics + manual title/poster

## What and why

User report: games still not properly recognized because of non-standard
directories. Ground truth from the user's `games.json` (every row on
`_placeholder.png`):

| Row | Root cause | General fix |
|---|---|---|
| Bramble (row = `…/IntermediateBuildDRM/WindowsNoEditor`) | UE packaged-build staging dirs are not engine folders, so the staging dir rows instead of the real root; the real exe sits 5 levels under the root (> `maxExeDepth` 4) | UE staging names become transparent engine folders; depth 4 → 5 |
| RSI Launcher (`resources/elevate.exe`) | the real `RSI Launcher.exe` is skip-token'd ("launcher"), so the elevate helper wins | `elevate` joins the skip tokens → the dir ends exe-less → no row (correct: no game there) |
| Witcher 3 GOTY, Riven, Jazz 2 CC | GOG goggame titles Steam's search rejects (edition strings) | manual title |
| Control | PE title junk ("ControlLauncher") | manual title |
| Zelda BotW, Spelunky HD | correct titles, absent/differently-named on Steam | manual title / poster |

All fixes are general (no per-title hacks, per the issue-025 instruction).

### 1. Discovery heuristics (discovery package)

- `engineFolderNames` gains UE packaged-build staging names:
  `windowsnoeditor`, `windowsclient`, `windowsserver`, plus a new prefix
  match for `intermediatebuild*` (e.g. IntermediateBuildDRM) — these never
  become rows and never make their parent a container, but stay walkable
  so their exes count as the parent's own.
- `maxExeDepth` 4 → 5 (Bramble's
  `IntermediateBuildDRM/WindowsNoEditor/<Game>/Binaries/Win64/game.exe`
  beats Prey's 4-level record by one).
- `recursiveSkipTokens` gains `elevate` (RSI/updater helper binary, never
  the game).

### 2. Set title manually (GUI + TUI)

UI for the existing `title_overrides` settings map:
`Session.SetTitleOverride(dir, title)` persists the override (empty
clears), renames the row in place (TitleSource `override`), and
re-resolves the cover with the new title. GUI: "Set title" affordance in
the detail panel with a pre-filled input; TUI: `t` on the detail screen,
prompt pre-filled, empty input clears.

### 3. Upload poster (GUI + TUI)

New `cover_overrides` settings map (canonical dir → cache filename),
mirroring the `title_overrides` precedent:

- `pickdir.PickFile` extends the native-dialog package with an
  image-filtered file picker (zenity/kdialog, PowerShell OpenFileDialog,
  osascript). GUI uses it; TUI takes a typed path.
- `Session.SetCoverOverride(dir, imagePath)`: validates the image
  decodes, **copies** it into the cover cache as
  `override_<sha256(dir)[:16]>.img` (originals untouched — a read-only
  pick, so no issue-023 backup gate), runs the 2:3 `normalizeCover`
  invariant (issue 025), records the override, updates the row
  immediately. Re-upload replaces (stable key).
- Precedence: `resolveCover` checks the override first — user art beats
  the entire fetch chain and survives rescans; a missing cache file
  falls through to the chain (self-healing).
- `Session.ClearCoverOverride(dir)` removes the map entry and the cached
  copy; the next cover resolution falls back to the chain.

## Acceptance

- [x] A Bramble-shaped fixture (`root/IntermediateBuildDRM/WindowsNoEditor/<Game>/Binaries/Win64/game.exe`)
      rows as `root` itself, not the staging dir.
- [x] A dir whose only exe is `elevate.exe` (real exe skip-token'd)
      produces no row.
- [x] Existing discovery tests stay green (no regressions from depth 5 /
      new engine names).
- [x] `SetTitleOverride` renames the row, persists `title_overrides`,
      re-resolves the cover; empty clears.
- [x] `SetCoverOverride` copies + normalizes the picked image into the
      cache, persists `cover_overrides`, and `resolveCover` prefers it
      over the fetch chain across rescans; `ClearCoverOverride` reverts.
- [x] `cover_overrides` survives a settings save/load round-trip
      (explicit raw-struct plumbing — the SteamGridDBKey lesson).
- [x] GUI: detail-panel "Set title" (inline input) and "Set poster…"
      (native dialog) / "Reset poster"; TUI: detail keys `t`/`a` (+ `A`
      reset) with prompts and action-list entries.
- [x] Full `go test ./...` green, `go vet`/`gofmt` clean,
      `GOOS=windows`/`darwin go build ./...` OK.
- [x] Docs: this issue, `docs/log.md`, `docs/architecture.md`,
      `docs/scope.md`, `README.md`.

## Outcome

Implemented as specced.

- **Heuristics** (discovery): `windowsnoeditor`/`windowsclient`/
  `windowsserver` joined `engineFolderNames` and `intermediatebuild*`
  became a prefix match in `engineFolderName`; `maxExeDepth` 4 → 5;
  `elevate` joined `recursiveSkipTokens`. The user's Bramble row now
  binds to the real game root (its PE title "Bramble - The Mountain
  King" feeds the cover search), and the RSI Launcher row disappears.
- **Set title**: `Session.SetTitleOverride` (internal/ui/overrides.go)
  persists `title_overrides`, renames the row in place (re-deriving via
  the identification chain on clear), and re-resolves the cover
  asynchronously. GUI: detail-panel "Set title" with an inline input +
  explicit Apply/Cancel (titleedit.go); TUI: `t` on the detail screen,
  pre-filled prompt, empty clears.
- **Upload poster**: `covers.SetOverride` validates decodability, copies
  into the cache as `override_<sha256(dir)[:16]>.img` (source untouched,
  stable name → re-upload replaces), and normalizes to 2:3;
  `OverridePath`/`ClearOverride` resolve only `override_*.img` basenames
  (settings is user-editable). `pickdir.PickFile` extends the native
  dialogs (zenity/kdialog image filter, IFileOpenDialog file mode via
  the shared COM path, osascript `choose file`). Precedence lives in the
  session's `coverOverride` helper, consulted by both `resolveCover` and
  `refreshCovers` — **the overrides map is passed in, never read via
  `s.Settings()` inside, because the scan settle calls `refreshCovers`
  while holding `s.mu`** (a self-deadlock shipped briefly in the first
  green run and hung every scan; caught by the full suite, fixed by
  snapshotting `coverPins` before the settle lock). `ClearCoverOverride`
  deletes entry + cache file and re-resolves through the chain. GUI:
  "Set poster…" (native dialog via `PickAndSetCover`) + "Reset poster"
  when a pin is active; TUI: `a` typed path, `A` reset.
- ATDD red witnessed: 3 behavioral failures (discovery layouts) + 5
  compile reds (settings/covers/ui/tui/gui) before implementation.
- Verification: `go test ./...` (30 packages) exit 0, `go vet`/`gofmt`
  clean, `GOOS=windows`/`darwin go build ./...` OK. Concurrent-session
  note: the foreign session's concurrent edits to
  internal/settings/settings_test.go (a duplicated SGDB block) and the
  issue-27 "Open game folder" button in internal/gui/view.go were left
  intact; only this issue's hunks are staged.
- Commit: `202e5ea`.
