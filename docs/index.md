# optiscaler-manager docs

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games. GUI: [go-shirei](https://github.com/hasenj/go-shirei)
(`go.hasen.dev/shirei`, pinned v0.5.2). Current release: v0.7, teaching
scanning the difference between a game folder and a library root —
manually added containers become scan roots whose games surface as
individual rows; v0.6 added
detection of pre-existing (external, unmanaged) OptiScaler installs by PE
version-info identity with adopt/refuse/restore semantics; multi-store
(Steam/Epic/GOG/manual), Linux + Windows builds.

## Document map

| File | OKF type | Contents |
|------|----------|----------|
| `log.md` | reserved | Milestone/task log, append-only |
| `scope.md` | reference | Scope by version (v0.1–v0.7), settled decisions, cut list |
| `architecture.md` | explanation | Package layout, data flow, startup cache, scan phases, external detection, game-dir classification, cross-platform shape, cancellation model |
| `safety.md` | explanation | Install invariants, manifest, rollback model, cancellation + launch safety |
| `plan.md` | reference | Milestone sequence, waves, verification gates |
| `vendor-patches.md` | reference | Local patches on vendored deps (shirei Wayland CSD), reapply procedure |

## Conventions (from AGENTS.md)

- TDD first: failing test before production code, behavior over plumbing.
- Verify with `go test ./...` only. Never `go run .`, never build the binary.
- `zerolog` in production code, `t.Log` in tests.
- Docs (README, docs/, log.md) updated before a task is considered done.
- Commit after each completed, fully-tested task.
- Ponytail minimalism: stdlib → platform → existing dep → one-liner → minimal code.
