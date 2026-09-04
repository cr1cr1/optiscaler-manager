---
type: explanation
---

# Safety model

The installer mutates directories owned by games. These invariants are
non-negotiable; the fault-injection tests in `internal/installer` exist to
keep them true.

## Invariants

1. **No write into a game directory until the bundle is staged, path-sanitized,
   required-file validated, and hash-manifested.**
2. **A manifest is persisted before the first destructive write.**
3. **An original file's bytes are verified in the backup before it is
   overwritten.**
4. **Rollback and uninstall are idempotent** — safe to re-run after a crash at
   any step.
5. **Uninstall deletes only files whose current SHA-256 matches the manifest**;
   anything else is refused and surfaced to the user (no silent deletion of
   foreign bytes — `dxgi.dll` et al. may be native game files).

## Manifest

JSON, external store, one per install (keyed by canonical install dir):

```
id, schemaVersion, status, gameRoot, installDir (canonical),
requestedVersion, resolved {assetName, version, sha256},
overwritten[] {path, backupRelPath, preSHA256, installedSHA256},
created[]    {path, sha256},
createdDirs[], timestamps, lastError
```

`requestedVersion` and `resolved` are separate fields: rate-limit or explicit
substitution must never silently change what the user asked for.

## State machine

```
in_progress → committed
           ↘ failed → rolled_back
```

`planned` is in-memory only. On startup, manifests in `in_progress`/`failed`
are surfaced with repair / rollback / retry choices; nothing is auto-deleted.
The surface per frontend: plain CLI commands print a stderr warning naming
the manifest (`cmd/checkInterrupted`, gated by `warnInterruptedCLI`); the
GUI shows a persistent banner under the toolbar and the TUI a warning line
above the games list, both derived from row state so cold and warm boots
are covered, plus a one-shot boot toast on the warm-cache path. In every
frontend the interrupted rows sort first and carry the actual choices: the
Rollback button restores, Install/QuickInstall retries.

## Backups

External per-install backup directory keyed by manifest ID, holding the
original bytes of every overwritten file (relative-path preserved). No
content-addressed store in v0.1 (dedup is not a correctness problem).

## Archive validation

Third-party archives are hostile input. Before any write: reject absolute
paths, `..` traversal, drive roots, UNC paths, reserved names, symlink/hardlink
metadata, case-folded duplicate targets, dir/file conflicts, decompression-bomb
totals, and unexpected filenames. Extract to staging; verify required files and
hashes; only then copy into the game dir.

## Fault-injection scope

Destructive fault injection (five tests):

1. `TestRejectsUnsafeArchive` — traversal/absolutes rejected at plan time.
2. `TestBacksUpBeforeOverwrite` — original bytes verified in backup first.
3. `TestRecordsCreatedAndOverwritten` — manifest reflects reality after commit.
4. `TestRollbackFromInProgress` — restore originals, delete matching created.
5. `TestUninstallRefusesChangedFile` — SHA mismatch → refuse, report.

Cancellation fault injection (three tests; see the invariant below):
`TestInstallCancelBeforeExtract_RollsBack`,
`TestInstallCancelMidSwap_ManifestFailedAndRollbackClean`,
`TestUninstallCancel_IdempotentResume`.

Deliberately absent: syscall-combinatoric fault matrices, content-addressed
backup stores, clock interfaces. Ceiling named: if real-world crash reports
appear, deepen injection at the observed boundary (`ponytail`).

## Cancellation invariant

6. **Cancelling an op at any phase boundary leaves zero partial state.** The
   manifest is marked `failed` with the context cause recorded, the install
   is rolled back to the pre-op state, and the returned error unwraps to
   `context.Canceled`.

Mechanics:

- Cancel checks sit at every phase boundary: pre-resolve and pre-download
  (`internal/app`), pre-extract, per-file during the swap, post-extract,
  post-INI write, and pre-commit (`internal/installer`), plus per-entry
  checks in the rollback/uninstall loops.
- Rollback triggered by a cancellation runs under `context.WithoutCancel`:
  the op context is dead by definition, but cleanup belongs to the same
  atomic operation and must run to completion. Detaching (rather than
  `context.Background()`) keeps the caller's values — trace IDs, log
  fields — on the cleanup path while ignoring cancellation.
- A cancelled install ends `rolled_back` (the usual `failed → rolled_back`
  transition); `last_error` records the cancellation.
- A cancelled uninstall keeps the manifest `committed` with processed
  entries already dropped; retrying resumes where it stopped.
- The session layer exposes per-game cancellation (`Session.CancelOp`); a
  cancelled op restores the row's pre-op status and surfaces exactly one
  "Cancelled" toast/event — never the failure path.

## Hook disable/enable rename

The disable toggle performs one write inside a game directory — a rename
of the install's injection hook. No file content is ever touched.

- **Disable renames only an identity-verified hook.** `pever.ActiveHook`
  parses the candidate PE for an OptiScaler marker before the rename, so
  a lookalike (DXVK's `dxgi.dll`) is refused with a toast; a foreign
  bytes-in-place file is never renamed.
- **Enable is presence-based on established installs.** A known hook name
  carrying the `.disabled` suffix (or a hand backup suffix like `.1` /
  `.bak`) means parked OptiScaler and is restored to its real hook name.
  The identity gate applies to parked hooks only at first-time detection
  of unmanaged directories (`pever.DisabledHookVerified`), where a lookalike
  `.disabled` file must not create an install.
- The parked state is display-only: the manifest is untouched, the
  install keeps its status, and re-enabling is a single atomic rename.
  An install whose hook the user renamed to a name outside the known
  candidate set is invisible to the toggle by design (same blind spot as
  the scanner) — the game would not load it either.

## Launch safety

Launching a game is fire-and-forget by design:

- Spawns are detached (`Start` + `Process.Release`, never `Wait`). The app
  holds no process handles, tracks no game lifetime, and never kills what it
  started. Logs say "launch requested", never "launched" — spawn success
  proves nothing about the game running.
- The manual-store template is split on double-quote grouping only: no shell,
  no metacharacter expansion. `{exe}`/`{dir}`/`{appid}`/`{args}` expand to
  argv entries verbatim, so a crafted game path cannot inject commands.
- URL openers (xdg-open / open / rundll32) get a 10-second context cap and
  are waited on, so a hung opener can't leak a zombie or block the session.
- Proton is never invoked directly (`proton run` is an upstream-unsupported
  path); Steam games launch through `steam://rungameid`, leaving Proton
  selection to Steam. The one carve-out is the opt-in umu-launcher
  integration on Linux: manual-store Windows binaries launch through
  `umu-run` (not raw `proton run`) with a deterministic per-game prefix
  under the state root, and the Proton build is user-pinnable via
  `UmuProtonPath` (empty = auto-detect from Steam/Bottles/umu runner
  dirs). The toggle defaults off; off-Linux the hook is nil and this path
  never fires.
