//go:build linux

package umu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/launchlog"
)

// Launcher is the runner seam. The default production value is built by
// defaultLauncher and executes cmd as-is. Tests inject a fake that
// captures cmd.Env / cmd.Args and returns canned stderr + exit codes.
type Launcher func(ctx context.Context, cmd *exec.Cmd) error

// defaultLauncher is the production runner: it executes cmd as-is,
// blocking until the child exits. Callers that need detachment inject
// their own Launcher (the seam exists for that and for tests).
func defaultLauncher(ctx context.Context, cmd *exec.Cmd) error { return cmd.Run() }

// Launch invokes umu-run with the supplied options. The child runs with
// a synthesized environment: parent env plus umu control vars (GAMEID,
// WINEPREFIX, PROTONPATH if set, STORE, PROTON_VERB=waitforexitandrun,
// UMU_LOG=1). UMU_LOG makes umu verbose; with LogDir set that verbosity
// costs nothing and fills the per-launch log. PROTONPATH is omitted
// entirely when empty so umu falls back to its default token.
//
// With opts.LogDir set, the child's full stdout and stderr are
// captured to a per-launch file (see internal/launchlog) headed by the
// exact argv and env delta; failure errors then name the file. stderr
// additionally keeps flowing into the in-memory buffer that drives the
// tail diagnostics below.
//
// umu's __main__ wraps execution in try/except BaseException, so fatal
// setup failures (no Proton, invalid PROTONPATH, offline first-run)
// surface as Python tracebacks on stderr combined with exit code 0.
// Launch works around this quirk by scanning stderr for known
// fatal-substring markers and wrapping such results as ErrUmuSetupFailed
// regardless of the exit code.
//
// A non-zero exit code with clean stderr (a game crash) is returned
// unwrapped — the caller can errors.As(*exec.ExitError) to inspect it.
func Launch(ctx context.Context, launcher Launcher, opts LaunchOpts) error {
	if launcher == nil {
		launcher = defaultLauncher
	}
	if opts.UmuRunPath == "" {
		return ErrUnavailable
	}
	if opts.ExePath == "" {
		return ErrMissingExe
	}

	gameID := opts.GameID
	if gameID == "" {
		gameID = "umu-default"
	}

	verb := opts.Verb
	if verb == "" {
		verb = "waitforexitandrun"
	}
	delta := []string{
		"GAMEID=" + gameID,
		"WINEPREFIX=" + opts.WinePrefix,
		"PROTON_VERB=" + verb,
		"STORE=" + opts.Store,
		"UMU_LOG=1",
	}
	if opts.ProtonPath != "" {
		delta = append(delta, "PROTONPATH="+opts.ProtonPath)
	}
	for k, v := range opts.ExtraEnv {
		delta = append(delta, k+"="+v)
	}
	env := append([]string{}, os.Environ()...)
	env = append(env, delta...)

	cmd := exec.CommandContext(ctx, opts.UmuRunPath,
		append([]string{opts.ExePath}, opts.Args...)...)
	cmd.Env = env
	// Supervision (issue 068): the child gets its own session so the
	// WHOLE tree (umu-run → pressure-vessel → wine → game) shares one
	// process group, and cancelling the context kills the group — the
	// default CommandContext Cancel kills only the direct child and
	// would orphan the game. WaitDelay bounds the wait for output
	// pipes a killed grandchild might hold open (wineserver
	// stragglers inside the container may still escape; see the issue
	// doc).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 10 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	logPath := ""
	if opts.LogDir != "" {
		p, f, err := launchlog.Create(opts.LogDir, opts.LogName)
		if err != nil {
			// Logging must never block a launch.
			log.Warn().Err(err).Str("dir", opts.LogDir).Msg("launch log unavailable")
		} else {
			defer f.Close()
			logPath = p
			launchlog.WriteHeader(f, cmd.Args, delta)
			cmd.Stdout = f
			cmd.Stderr = io.MultiWriter(f, &stderr)
		}
	}

	runErr := launcher(ctx, cmd)

	tail := stderrTail(stderr.String())
	if isFatalUmuFailure(stderr.String()) {
		if runErr == nil {
			runErr = errors.New("umu: fatal stderr detected with clean exit")
		}
		return errors.Join(ErrUmuSetupFailed, withLogPath(withStderrTail(runErr, tail), logPath))
	}
	if runErr != nil {
		return withLogPath(withStderrTail(runErr, tail), logPath)
	}
	return nil
}

// withLogPath appends the per-launch log file to err so a toast or
// console error tells the user where the full diagnostics live. An
// empty path (logging disabled) leaves err untouched.
func withLogPath(err error, path string) error {
	if err == nil || path == "" {
		return err
	}
	return fmt.Errorf("%w\nfull log: %s", err, path)
}

// withStderrTail wraps err with umu's stderr tail so the real failure
// reason (missing Proton, bad PROTONPATH, Python traceback) survives
// into logs and toasts instead of vanishing with the child. The tail is
// empty for clean runs, leaving err untouched.
func withStderrTail(err error, tail string) error {
	if tail == "" {
		return err
	}
	return fmt.Errorf("%w\numu stderr tail:\n%s", err, tail)
}

const (
	// stderrTailLines bounds how many trailing stderr lines diagnostics
	// keep; stderrTailCap bounds the total size.
	stderrTailLines = 10
	stderrTailCap   = 1024
)

// stderrTail returns the last stderrTailLines non-empty lines of s,
// capped at stderrTailCap bytes (truncated from the front).
func stderrTail(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > stderrTailLines {
		lines = lines[len(lines)-stderrTailLines:]
	}
	out := strings.Join(lines, "\n")
	if len(out) > stderrTailCap {
		out = out[len(out)-stderrTailCap:]
	}
	return out
}

// fatalMarkers are substrings that reliably indicate an umu setup
// failure in stderr. Tracebacks are the universal signal; the rest are
// documented in the umu source (umu_run.py raises FileNotFoundError /
// RuntimeError / ValueError at known failure sites). "ConnectionError"
// is intentionally NOT a fatal marker — a transient network failure
// during a one-off download should not be classified as setup failure.
var fatalMarkers = []string{
	"Traceback (most recent call last)",
	"FileNotFoundError",
	"RuntimeError",
	"ValueError",
	"umu has not been setup",
	"PROTONPATH is not valid",
	"Environment variable not set or is empty: PROTONPATH",
}

func isFatalUmuFailure(stderr string) bool {
	if stderr == "" {
		return false
	}
	for _, m := range fatalMarkers {
		if strings.Contains(stderr, m) {
			return true
		}
	}
	return false
}
