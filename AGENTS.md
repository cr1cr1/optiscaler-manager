# Local Agent Instructions

Before following any other rules in this file, load and apply the global
`AGENTS.md` at `~/.agents/AGENTS.md`.

On top of the global rules:

1. **TDD first**: For every behavioral change, write or extend a failing test
   before the production code. Tests must cover behavior, not plumbing.
2. **Update all docs**: Any change that affects user-facing docs, README,
   `docs/`, milestones, tasks, or examples must update them before finishing.
   Keep `docs/log.md` current with completed milestones and tasks.
3. MUST follow OKF format: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md

When in doubt, prefer deletion, simplicity, and the documented project conventions
in `docs/`.

## Guidelines

  - DO NOT build the binary, run with `go run .`, or use `go test -run` to skip tests. Always run `go test ./...` to verify all tests pass. You can use `go vet` to check the code.
  - Use `zerolog` for logging in production code. Use `t.Log` in tests.
  - `git commit` after each task is complete and all tests pass. Do not commit half-done work unless the user instructs so.
