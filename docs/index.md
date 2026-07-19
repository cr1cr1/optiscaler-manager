# optiscaler-manager docs

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games. GUI: [go-shirei](https://github.com/hasenj/go-shirei)
(`go.hasen.dev/shirei`, pinned v0.5.2). Platform target for v0.1: Linux + Steam (Proton).

## Document map

| File | OKF type | Contents |
|------|----------|----------|
| `log.md` | reserved | Milestone/task log, append-only |
| `scope.md` | reference | v0.1 scope, settled decisions, cut list |
| `architecture.md` | explanation | Package layout, data flow, GUI boundary |
| `safety.md` | explanation | Install invariants, manifest, rollback model |
| `plan.md` | reference | Milestone sequence, waves, verification gates |

## Conventions (from AGENTS.md)

- TDD first: failing test before production code, behavior over plumbing.
- Verify with `go test ./...` only. Never `go run .`, never build the binary.
- `zerolog` in production code, `t.Log` in tests.
- Docs (README, docs/, log.md) updated before a task is considered done.
- Commit after each completed, fully-tested task.
- Ponytail minimalism: stdlib → platform → existing dep → one-liner → minimal code.
