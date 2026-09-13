# CLI Surfaces (v0.16) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the post-v0.1 features (version switch incl. `latest`, DLSS update/restore, launch, hook toggle) as one-shot CLI commands over the shared `ui.Session`.

**Architecture:** New kong commands in `cmd/` build the same `ui.NewSession` the GUI/TUI use, dispatch the op, and block on a small CLI-side waiter that drains `Session.Events()` (buffered chan, cap 64) until `EvOpDone`/`EvOpFailed`/`EvOpCancelled` for the target dir. Consent gates arrive as `EvConfirm`; the waiter prompts y/n on the terminal and declines when stdin is not interactive — the safety model is never bypassed. No core (`internal/ui`, `internal/gh`, `internal/installer`) changes except two constructor seams (`Deps.DLSS`, `Deps.Launcher`) that mirror the existing `Deps.GH` pattern so commands are testable.

**Tech Stack:** Go 1.26, kong CLI, zerolog (production) / `t.Log` (tests), stdlib only.

**Spec:** the user contract lands in `docs/scope.md` → `## v0.16 scope (CLI surfaces)` (Task 5); design decisions below. Stable-only tracking stays; nightly repo explicitly dropped.

## Global Constraints

- TDD first: every behavioral change gets its failing test written and watched red before the code.
- Verify with `GOCACHE=$PWD/tmp/gocache go vet ./...` and full `GOCACHE=$PWD/tmp/gocache go test -count=1 ./...` (never `go test -run`, never `go run .`); lint `GOPATH=$PWD/tmp/gopath GOCACHE=$PWD/tmp/gocache golangci-lint run ./...` must print `0 issues.`; race on touched packages.
- zerolog in production code, `t.Log` in tests. Docs (OKF) updated in the same change. One commit at the end (item-level), lowercase imperative subject, no co-author tags.
- Consent is never bypassed: no `--yes` flag exists anywhere.
- Exit codes: success 0, runtime failure 1 (via `ExitError`), usage/parse 2 (existing kong wiring).
- Ops reuse the session's per-game busy/cancel slot; a busy target op fails fast with its existing refusal semantics.

---

### Task 1: Deps seams for DLSS and launcher (testability, mirrors GH)

**Files:**
- Modify: `cmd/deps.go` (Deps struct + newDeps)
- Modify: `cmd/session.go` (newSession uses the seams when set)
- Test: none needed yet (pure wiring, exercised by Tasks 2–5 tests)

**Interfaces:**
- Produces: `Deps.DLSS dlss.Client` (interface type used by `ui.Deps.DLSS` — check its exact type in `internal/ui/session.go` Deps), `Deps.Launcher launch.Launcher`; `newSession` prefers them when non-nil.

- [ ] **Step 1: Inspect the exact types** — `ui.Deps.DLSS` and `ui.Deps.UmuLauncher` field types in `internal/ui/session.go:130-165`; `cmd/deps.go` Deps fields.
- [ ] **Step 2: Add the two fields** to `Deps` with doc comments naming the pattern ("nil → built in newSession, like GH").
- [ ] **Step 3: Wire newSession** — `dlssClient := d.DLSS; if dlssClient == nil { dlssClient = dlss.New(httpClient) }` (same shape for the launcher vs `newUmuLauncher(prefs)`).
- [ ] **Step 4: `GOCACHE=$PWD/tmp/gocache go vet ./cmd/`** — expect clean.

### Task 2: op waiter + consent prompt (`cmd/opwait.go`)

**Files:**
- Create: `cmd/opwait.go`
- Test: `cmd/opwait_test.go`

**Interfaces:**
- Consumes: `ui.Session.Events() <-chan ui.Event`, `Event{Kind, Text, GameDir}`, `EvOpDone/EvOpFailed/EvOpCancelled/EvConfirm`, `Session.Snapshot().Confirm *Confirmation{Kind, GameDir, Message}`, `Session.AnswerConfirm(bool)`, `Session.QuickInstall(string)`, `Session.Start(context.Context)`, `Session.Snapshot().Rows []ui.GameRow`.
- Produces: `waitForOp(sess *ui.Session, dir string, timeout time.Duration) (ui.Event, error)` — terminal op event for `dir` (ignores other dirs' events), or error on timeout/channel-close; answers confirm gates inline.

- [ ] **Step 1: Write the failing tests** (`cmd/opwait_test.go`):

```go
// TestWaitForOpReturnsDone drives a real QuickInstall through a session
// built the way the commands build it and asserts the waiter returns the
// done event with the install landed on disk.
func TestWaitForOpReturnsDone(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	sess := newSession(d)
	sess.Start(context.Background())
	pollForRows(t, sess, 1)

	ev, err := waitForOp(sess, gameRoot, 30*time.Second)
	go func() { sess.QuickInstall(gameRoot) }() // order fixed below; see step note
	_ = ev; _ = err; _ = out; _ = steamRoot
}
```

Actual form (dispatch BEFORE waiting, matching real use):

```go
func TestWaitForOpReturnsDone(t *testing.T) {
	srv := fakeGitHub(t)
	_, gameRoot := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	sess := newSession(d)
	sess.Start(context.Background())
	pollForRows(t, sess, 1)

	sess.QuickInstall(gameRoot)
	ev, err := waitForOp(sess, gameRoot, 30*time.Second)
	if err != nil {
		t.Fatalf("waitForOp: %v", err)
	}
	if ev.Kind != ui.EvOpDone {
		t.Fatalf("event kind = %v, want EvOpDone (%q)", ev.Kind, ev.Text)
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "bin", "dxgi.dll")); err != nil {
		t.Fatalf("install did not land files: %v", err)
	}
	t.Logf("done event: %q", ev.Text)
}

// pollForRows is the cmd-side pollUntil (mirror of the tui/ui helpers).
func pollForRows(t *testing.T, sess *ui.Session, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for len(sess.Snapshot().Rows) != n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d rows (have %d)", n, len(sess.Snapshot().Rows))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestWaitForOpNonInteractiveDeclinesConsent: an EAC-protected game pauses
// on ConfirmEAC; stdin is not a terminal under go test, so the waiter must
// decline, the op must abort, and no file may land.
func TestWaitForOpNonInteractiveDeclinesConsent(t *testing.T) {
	srv := fakeGitHub(t)
	_, gameRoot := fakeSteam(t)
	writeCmdTestFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	sess := newSession(d)
	sess.Start(context.Background())
	pollForRows(t, sess, 1)

	sess.QuickInstall(gameRoot)
	ev, err := waitForOp(sess, gameRoot, 30*time.Second)
	if err == nil {
		t.Fatal("waitForOp must fail when consent is declined non-interactively")
	}
	if !strings.Contains(err.Error(), "consent") && ev.Kind != ui.EvOpCancelled && ev.Kind != ui.EvOpFailed {
		t.Fatalf("err = %v, event = %+v; want a refusal or the cancel event", err, ev)
	}
	if _, serr := os.Stat(filepath.Join(gameRoot, "bin", "dxgi.dll")); !os.IsNotExist(serr) {
		t.Fatal("install proceeded without consent")
	}
	t.Logf("declined non-interactive consent: err=%v event=%v %q", err, ev.Kind, ev.Text)
}
```

- [ ] **Step 2: Run red** — `GOCACHE=$PWD/tmp/gocache go test -count=1 ./cmd/` → FAIL: `waitForOp` undefined (both tests).
- [ ] **Step 3: Implement** (`cmd/opwait.go`):

```go
package optiscalermanager

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// waitForOp blocks until the session finishes, refuses, or cancels the op
// for gameDir — the CLI's equivalent of the GUI/TUI event loop. Consent
// gates (EvConfirm) are answered on the terminal; a non-interactive stdin
// declines instead of bypassing consent (docs/safety.md). Events for other
// game dirs are ignored.
func waitForOp(sess *ui.Session, dir string, timeout time.Duration) (ui.Event, error) {
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-sess.Events():
			if ev.GameDir != "" && ev.GameDir != dir {
				continue
			}
			switch ev.Kind {
			case ui.EvOpDone, ui.EvOpFailed, ui.EvOpCancelled:
				return ev, nil
			case ui.EvConfirm:
				promptConfirm(sess)
			}
		case <-deadline:
			return ui.Event{}, fmt.Errorf("timeout after %s waiting for the operation on %s", timeout, dir)
		}
	}
}

// promptConfirm renders the pending confirmation and answers it from the
// terminal; a non-interactive stdin declines (never bypasses consent).
func promptConfirm(sess *ui.Session) {
	c := sess.Snapshot().Confirm
	if c == nil {
		return
	}
	interactive := false
	if fi, err := os.Stdin.Stat(); err == nil {
		interactive = fi.Mode()&os.ModeCharDevice != 0
	}
	if !interactive {
		fmt.Fprintf(os.Stderr, "refused: %s requires consent and stdin is not interactive\n", c.Message)
		sess.AnswerConfirm(false)
		return
	}
	fmt.Fprintf(os.Stderr, "%s\nproceed? [y/n] ", c.Message)
	ans, err := bufio.NewReader(os.Stdin).ReadString('\n')
	accept := err == nil && func() bool {
		a := strings.ToLower(strings.TrimSpace(ans))
		return a == "y" || a == "yes"
	}()
	if !accept {
		fmt.Fprintln(os.Stderr, "declined")
	}
	sess.AnswerConfirm(accept)
}
```

- [ ] **Step 4: Run green** — both tests pass; no other cmd test breaks.
- [ ] **Step 5: Refactor check** — no duplication with internal packages (the waiter is cmd-owned by design: the GUI/TUI have their own loops).

### Task 3: `switch` command (version switching incl. `latest`)

**Files:**
- Create: `cmd/switch.go`
- Test: `cmd/switch_test.go`

**Interfaces:**
- Consumes: `waitForOp`, `newSession`, `Session.SwitchVersion(gameDir, version)` (async; literal `"latest"` resolves at pick time — v0.15 seam, no new core logic), `Session.Settings().DefaultVersion`, `Session.Snapshot().Rows` (row `OptiScalerVersion`), `ui.ErrRateLimited` (via `errors.Is` on the terminal event? no — op failures surface as `EvOpFailed`).
- Produces: `SwitchCmd{Path string; Version string; Timeout time.Duration}` with `Run(d *Deps) error` printing `switched <title> → <tag>` and returning `&ExitError{Code: 1, Err: …}` on failure.

- [ ] **Step 1: Failing tests** (`cmd/switch_test.go`):

```go
// TestSwitchCommandResolvesLatest: install the fixture, then switch with
// Version:"latest" — the row must land on the resolved tag (v0.9.4-test,
// the fake's only non-prerelease), proving pick-time resolution.
func TestSwitchCommandResolvesLatest(t *testing.T) {
	srv := fakeGitHub(t)
	_, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	sess := newSession(d)
	sess.Start(context.Background())
	pollForRows(t, sess, 1)
	sess.QuickInstall(gameRoot)
	pollForCommitted(t, sess, gameRoot)

	cmd := &SwitchCmd{Path: gameRoot, Version: "latest"}
	if err := cmd.runWith(sess, d); err != nil {
		t.Fatalf("SwitchCmd: %v", err)
	}
	row := rowOf(t, sess, gameRoot)
	if row.OptiScalerVersion != "v0.9.4-test" {
		t.Errorf("version after latest switch = %q, want v0.9.4-test", row.OptiScalerVersion)
	}
	if !strings.Contains(out.String(), "switched") || !strings.Contains(out.String(), "v0.9.4-test") {
		t.Errorf("output missing the switch report:\n%s", out.String())
	}
}

// TestSwitchCommandConcreteTag: Version:"v0.9.4-test" on a row already at
// v0.9.4-test must no-op with a clear message and exit 0 (S13 parity).
func TestSwitchCommandSameVersionNoOp(t *testing.T) { … same fixture … 
	cmd := &SwitchCmd{Path: gameRoot, Version: "v0.9.4-test"} // already installed
	if err := cmd.runWith(sess, d); err != nil {
		t.Fatalf("same-version switch must not error: %v", err)
	}
	if strings.Contains(out.String(), "switched") {
		t.Errorf("same-version switch must not claim a switch:\n%s", out.String())
	}
}
```

Helpers `pollForCommitted(t, sess, dir)` and `rowOf(t, sess, dir)` live in `cmd/opwait_test.go` (cmd-package helpers, mirror the tui/ui poll pattern).

- [ ] **Step 2: Run red** — `SwitchCmd` undefined.
- [ ] **Step 3: Implement** (`cmd/switch.go`):

```go
package optiscalermanager

import (
	"fmt"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// SwitchCmd switches a game's OptiScaler version through the shared
// session core. Version "latest" is re-resolved at pick time by the core
// (v0.15 seam); an empty version means the configured default.
type SwitchCmd struct {
	Path    string `arg:"" help:"Game root directory" type:"path"`
	Version string `help:"Version tag to install, or 'latest' (re-resolved at pick time; default: the configured default version)"`
	Timeout time.Duration `help:"Max wait for the operation" default:"10m"`
}

// Run boots a session, dispatches the switch, and waits for the outcome.
func (c *SwitchCmd) Run(d *Deps) error {
	sess := newSession(d)
	sess.Start(cmdContext())
	return c.runWith(sess, d)
}

// runWith is the testable body: session supplied by the caller.
func (c *SwitchCmd) runWith(sess *ui.Session, d *Deps) error {
	version := c.Version
	if version == "" {
		version = sess.Settings().DefaultVersion
	}
	before := rowOfSession(sess, c.Path)
	sess.SwitchVersion(c.Path, version)
	ev, err := waitForOp(sess, c.Path, c.Timeout)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if ev.Kind == ui.EvOpFailed {
		return &ExitError{Code: 1, Err: fmt.Errorf("%s", ev.Text)}
	}
	after := rowOfSession(sess, c.Path)
	if after.OptiScalerVersion == before.OptiScalerVersion {
		fmt.Fprintf(d.Out, "already at %s (%s)\n", after.OptiScalerVersion, after.Title)
		return nil
	}
	fmt.Fprintf(d.Out, "switched %s → %s\n", after.Title, after.OptiScalerVersion)
	return nil
}
```

Notes: `rowOfSession(sess, dir)` (cmd helper, `opwait_test.go` or a small `cmd/session_helpers.go` — put it next to `newSession` in `cmd/session.go` so production and tests share it) returns the row or an error if the dir is unknown; `cmdContext()` returns `context.Background()` (defined once in `cmd/session.go`). Empty-row/unknown-dir handling: `rowOfSession` error → `ExitError{1}` before dispatch. EvOpCancelled → exit 1 with ev.Text.

- [ ] **Step 4: Green** — both switch tests pass.
- [ ] **Step 5: Gate** — `go vet ./cmd/`.

### Task 4: `dlss-update` and `dlss-restore` commands

**Files:**
- Create: `cmd/dlss.go`
- Test: `cmd/dlss_test.go`

**Interfaces:**
- Consumes: `Session.UpdateDLSS(gameDir)` (async), `Session.DLSSSnapshots(gameDir) []dlss.Snapshot` (sync, newest first; fields `ID string`, `Label() string`), `Session.RestoreDLSS(gameDir, snapshotID)` (async), the Task-1 `Deps.DLSS` seam, `waitForOp`.
- Produces: `DLSSUpdateCmd{Path string; Timeout}`, `DLSSRestoreCmd{Path string; Snapshot string; Timeout}`.

- [ ] **Step 1: Failing tests** (`cmd/dlss_test.go`) — mirror `internal/tui/dlss_test.go`'s fixture approach for the NVIDIA fake: that helper builds a dlss client against an httptest server (read `internal/tui/dlss_test.go:27-76` `dlssEnv` for the exact fake endpoints and DLL writes; replicate the fixture in cmd with `writeCmdTestFile`, constructing the dlss client the same way and passing it via the new `Deps.DLSS` seam). Tests:
  - `TestDLSSUpdateCommandUpdatesRuntime`: seeded three-DLL game with old versions → `DLSSUpdateCmd.Run` → waiter done → the three DLLs at the fake's published version; output line `updated NVIDIA DLSS runtime` + version.
  - `TestDLSSRestoreCommandRestoresSnapshot`: seed a snapshot via the dlss client update flow (as the tui test does), then `DLSSRestoreCmd{Snapshot: "<id>"}` → done → DLLs back at the snapshot's version; `Snapshot: ""` → newest snapshot restored.
- [ ] **Step 2: Run red** — both command types undefined.
- [ ] **Step 3: Implement** (`cmd/dlss.go`) — same body shape as `SwitchCmd.runWith`:

```go
// DLSSUpdateCmd updates the game's NVIDIA runtime through the session core.
type DLSSUpdateCmd struct {
	Path    string        `arg:"" help:"Game root directory" type:"path"`
	Timeout time.Duration `help:"Max wait for the operation" default:"10m"`
}

func (c *DLSSUpdateCmd) Run(d *Deps) error {
	sess := newSession(d)
	sess.Start(cmdContext())
	return c.runWith(sess, d)
}

func (c *DLSSUpdateCmd) runWith(sess *ui.Session, d *Deps) error {
	sess.UpdateDLSS(c.Path)
	ev, err := waitForOp(sess, c.Path, c.Timeout)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if ev.Kind == ui.EvOpFailed {
		return &ExitError{Code: 1, Err: fmt.Errorf("%s", ev.Text)}
	}
	fmt.Fprintf(d.Out, "%s\n", ev.Text)
	return nil
}

// DLSSRestoreCmd restores a previously snapshotted NVIDIA DLL set. An empty
// Snapshot means the newest snapshot.
type DLSSRestoreCmd struct {
	Path     string        `arg:"" help:"Game root directory" type:"path"`
	Snapshot string        `help:"Snapshot id (default: newest)"`
	Timeout  time.Duration `help:"Max wait for the operation" default:"10m"`
}
```

`DLSSRestoreCmd.runWith`: resolve the id — empty → `sess.DLSSSnapshots(c.Path)[0].ID` (error `ExitError{1, "no DLSS snapshots for <path>"}` when the list is empty); then `sess.RestoreDLSS`, wait, print `ev.Text`. Restore runs through the session's `ConfirmDLSSRestore` gate — the waiter's prompt handles it (non-interactive tests decline; the happy-path restore test must simulate consent: drive `sess.AnswerConfirm(true)` from a goroutine after staging — see `internal/ui/dlss_test.go` for how the ui tests consent; the cmd test uses the same pattern with the waiter running in a goroutine: `go func() { sess.Start…; sess.RestoreDLSS… }()` then loop `waitForOp` with the accept goroutine, or set the fake stdin to a pipe that yields "y\n" by swapping `os.Stdin` via a test-only seam — prefer the explicit `promptConfirm` seam: make `promptConfirm` a package var `promptConfirmFn = promptConfirm` so tests inject an acceptor. That is ONE seam, documented, replacing stdin fiddling).

- [ ] **Step 4: Green** — both dlss command tests pass.
- [ ] **Step 5: Gate** — `go vet ./cmd/`.

### Task 5: `launch` and `hook` commands

**Files:**
- Create: `cmd/launch.go`, `cmd/hook.go`
- Test: `cmd/launch_test.go`, `cmd/hook_test.go`

**Interfaces:**
- Consumes: `Session.Launch(gameDir)` (async; done/failed events carry GameDir), Task-1 `Deps.Launcher` seam, `Session.ToggleDisabled(gameDir)` (SYNCHRONOUS rename — no waiter), `Session.Snapshot().Rows` `Disabled` field.
- Produces: `LaunchCmd{Path string}`, `HookCmd{Path string; Enable bool; Disable bool}` (exactly one of Enable/Disable required — kong `xor`).

- [ ] **Step 1: Failing tests**:
  - `TestLaunchCommandRunsRunner`: inject a capturing launcher via `Deps.Launcher` (pattern: `internal/tui/model_test.go` `launchCapture`) → `LaunchCmd.Run` → done event → argv contains `steam://rungameid/100`; output `launch requested: Game One`.
  - `TestHookCommandTogglesDisabled`: committed row → `HookCmd{Enable:false|Disable:true}` via xor fields → row `Disabled` true → output `disabled OptiScaler hook for Game One`; second run `Enable:true` → false → output `enabled …`.
- [ ] **Step 2: Run red.**
- [ ] **Step 3: Implement** — `LaunchCmd`: session + `sess.Launch(c.Path)` + `waitForOp` + print `ev.Text` (failed → exit 1). `HookCmd`: session + validate exactly one of Enable/Disable (kong `xor:"hookstate"` on both fields) + `sess.ToggleDisabled(c.Path)` + read the row + print; NO waiter (sync op), no `--timeout`.

- [ ] **Step 4: Green** — both tests pass.

### Task 6: docs, gates, review, commit

**Files:**
- Modify: `docs/scope.md` (new `## v0.16 scope (CLI surfaces)` section: the five commands, consent model, exit codes, timeout default; note stable-only stays / nightly dropped), `docs/architecture.md` (new `## CLI surfaces (v0.16)` section after the shared-dropdown section: session-backed one-shot ops, the waiter, consent on stdin, no --yes), `README.md` (Commands list: the five new commands with one-line descriptions), `docs/log.md` (append entry), `docs/index.md` (mention CLI ops in the intro if it lists surfaces).
- No repo file outside cmd/ + docs/ changes.

- [ ] **Step 1: Write the doc edits** (OKF section style, ISO date headings, prose matching implemented behavior — every claim traceable to a command).
- [ ] **Step 2: Full gates** — vet, uncached full suite (`29/29` expected plus new cmd tests), race `./cmd/ ./internal/ui/`, lint `0 issues.`
- [ ] **Step 3: Two-axis /code-review** (Standards + Spec subagents on the working diff); address findings.
- [ ] **Step 4: Commit** — `feat: add one-shot CLI commands for switch, dlss, launch, and hook ops`; body lists the commands, the waiter + consent model, the two Deps seams, and the red-proofed tests. Include the plan doc `docs/superpowers/plans/2026-09-13-cli-surfaces.md` in the commit.

## Self-Review

- Spec coverage: five commands (Tasks 3–5), waiter+consent (Task 2), testability seams (Task 1), docs+gates+review (Task 6). The `--json` scripting mode and settings commands are NOT in scope (Option A decision, recorded in scope.md).
- Placeholder scan: Task 4 tests reference the tui dlss fixture by path and instruct replication — acceptable because the exact fake endpoints live in `internal/tui/dlss_test.go:27-76` and the plan says WHAT to replicate (endpoints, DLL writes, snapshot seeding); no "TBD".
- Type consistency: `waitForOp(sess, dir, timeout)` used identically in Tasks 2–5; `runWith(sess, d)` shape identical across commands; `pollForRows/pollForCommitted/rowOf` defined once (Task 2/3) and consumed later.
