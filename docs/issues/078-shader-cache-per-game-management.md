---
title: Per-game shader cache management in the details pane
description: KISS shader-cache compilation management per game (clean-room from SCSKiller concepts): discovery of known cache files plus global driver caches, clear with consent, env presets over the existing env store, and launch-and-record warm behind a compiler-backend seam; fossilize-replay deferred to a follow-up after host-GPU validation.
---

# 078 — Per-game shader cache management in the details pane

## What and why

SCSKiller (Windows/C#) manages shader-cache compilation per game: cache
discovery, status, warm/compile actions, staleness prompts. Clean-room
takeaway for optiscaler-manager (Linux/Proton, Go): only the *product
shape* transfers — per-game discovery → status → one or two actions.
Its engine (pipeline pre-compilation, warm states, community plan DB,
`%LOCALAPPDATA%\D3DSCache` parsing) is Windows-only and stays behind.

Directive: KISS, in the details pane, covering all caches that reduce
stutter — status + clear (a), env toggles (b), and pre-warm (c).

Spike (fossilize-replay feasibility, read-only, throwaway): `fossilize_replay`
ships with Steam (`ubuntu12_64/`) and runs; real caches confirmed
(per-game `cache/0000_0004_0002` 1.3 GB; shared Mesa 190 MB + db 32 MB,
NVIDIA GLCache 2.5 GB; Steam `shadercache/<appid>` empty — precache off).
Not provable in sandbox (no `/dev/dri`, no driver): replay compiles for
the *active* Vulkan driver (proprietary NVIDIA here, not RADV — win
unmeasured) and needs per-game recorded archives first. Verdict: viable
instrument, unvalidated backend — launch-and-record warms everywhere on
day one, fossilize stays a reserved backend for a follow-up issue.

Approaches considered: (A) thin pane over existing seams — recommended,
approved; (B) SCSKiller-faithful daemon with DB/notifications — rejected,
anti-KISS; (C) shell-out-to-external-tools only — rejected, Warm would
not exist for non-Steam games.

## Design (approved §1–§7)

§1 Discovery (`internal/shadercache`, pure, GPU-free). Output
`CacheRef{Kind, Scope, Path, SizeBytes, ModTime}`; `Kind` ∈ DXVKState
(`*.dxvk-cache`), VKD3D, SteamFoz (read-only), MesaGlobal, NvidiaGlobal;
inputs game dir + prefix dir + home dir (parameter for tests). Safety
boundary: v1 claims only exact-known filenames; opaque engine dirs
(e.g. Resonance `cache/0000_0004_0002`) are `Unclaimed` rows —
size shown, never cleared, never warmed-against. Unreadable → `Unknown`,
scan never fails.

§2 Pane UI. One collapsible section in the details panel reusing
existing components: per-cache rows (location, size, age, Clear) only
when present; global rows greyed with "shared by all games"; `Unclaimed`
info-only; `[Clear all game caches]`; env preset row; `[Warm now]` →
progress in MB + elapsed + `[Cancel]`. No pipeline counts (Linux offers
none — not faked). GUI first; TUI follows in the same issue only if its
renderer shares the row model cheaply, else a follow-up.

§3 Clear safety. Claimed per-game paths only (`os.RemoveAll` on explicit
paths); global Clear behind the existing `Confirmation` modal (new
`ConfirmKind`, zero plumbing changes); hard guards — never `Unclaimed`,
disabled while `Playing`, Clear locked mid-Warm; per-row warnings, one
stubborn file never blocks the rest.

§4 Env toggles. Preset-writer over the existing per-game env store
(`envedit.go` owns persistence): `DXVK_STATE_CACHE`, `__GL_SHADER_DISK_CACHE`,
`MESA_SHADER_CACHE_MAX_SIZE` selector. Off stops growth, never deletes
(said inline). No path-redirect vars (would break §1 discovery;
hand-set redirects show "custom path — not tracked"). Driver mismatch
is display-only.

§5 Warm lifecycle (launch-and-record). Guards (not Playing, no active
run, trackable location exists) → launch via the game's existing
launch route (game opens normally — pane says so) → watchdog polls
claimed paths (~5 s): completion on quiescence (3× quiet) or cap
(~20 min constant in v1) → existing stop path (graceful, then kill) →
toast `Warmed: +X MB in M:SS`. Cancel keeps partial gains. Launch
failure = error toast, no run; game crash ends run honestly.

§6 Backend seam. `CompilerBackend`: `Warm(ctx, game, onProgress)
(WarmResult, error)`; `WarmResult{BytesGrown, Elapsed, StopReason}`.
v1 ships `LaunchWatchBackend` only — no dead fossilize stub (ponytail);
fossilize-replay validation on host GPU is a filed follow-up, shaped to
the same result type. Steam precache stays a read-only status row.

§7 Testing. TDD red-first, `go test ./...`, no GPU: temp-dir discovery
fixtures (incl. unclaimed-untouched, unreadable→Unknown), existing
`detailpanel_test.go` patterns for rows/button states, watchdog with
injectable interval + fake backend.

## Acceptance

- [x] Details pane shows a shader-cache section per game: known caches
      with size/age/Clear, globals greyed-shared, unclaimed info-only.
- [x] Per-game Clear removes only claimed files; global Clear asks via
      the confirm modal; decline leaves bytes untouched.
- [x] Env presets persist through the existing env store and surface in
      the env editor; off never deletes.
- [x] Warm launches via the game's launch route, reports MB + elapsed,
      ends on quiescence/cap/cancel, stop reuses the Stop path.
- [x] `CompilerBackend` seam in place with `LaunchWatchBackend`;
      fossilize follow-up issue filed.
- [x] `go test ./...`, `go vet`, `gofmt` green; scope/architecture docs
      updated if decisions settle there.

## Outcome

Implemented as designed, in slices: `d04fd9e` discovery package,
`f4e99db` session ops/consent/env/warm-backend, `ae8de8e` GUI section
+ TUI status + `Deps.HomeDir` seam, `d7e509a` backend unit tests,
`e286250` review fixes. Ponytail-review accepted 2 findings: the
watchdog now measures game-only (no 5s home-tree re-walks) and the
consented shared clear re-validates the path at answer time (TOCTOU);
rest confirmed no-change (stop ordering, claimed-only deletes,
memo discipline, discovery walk).

Decisions on the plan's open points: Warm refuses fire-and-forget
routes (`shouldRouteWinBinary` default — no supervised slot, no
watchdog); TUI ships read-only status (actions need keybindings —
follow-up); no dead fossilize stub shipped. Left open: issue 080
(fossilize-replay validation on host GPU), TUI warm/clear keys.
 Reds witnessed per slice (missing package, undefined section
rects, toggle-state expectation corrected to checkbox semantics,
nested-claim fix, hermetic-HOME leak of the real 2.6 GB GLCache
into session tests).
- Commit: e286250.
- Pin: e286250.
