// Package termopen opens a text file in the user's terminal editor, running
// inside a terminal emulator, detached from this process.
//
// Resolution order: the editor is $EDITOR verbatim (split with
// strings.Fields — no shell quoting), else the first of micro, nano, vi on
// PATH. The terminal is $TERMINAL (its basename selects the run-a-command
// convention; unknown basenames get the -e best-effort), else the known
// chain foot→konsole→gnome-terminal→kitty→alacritty→xterm probed via
// lookPath. Open tries each terminal candidate in order: a spawn failure
// falls through to the next candidate. The platform default runner spawns
// detached (Setsid + Start + Release, never Wait); no shell is involved —
// argv is exec'd directly.
//
// Note the asymmetry between the two env vars: arguments in $EDITOR ARE
// supported (split with strings.Fields), but arguments in $TERMINAL are
// NOT — a value like "foot -s" is passed verbatim as argv[0], fails to
// spawn, and silently falls through to the next candidate.
package termopen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
)

// Sentinel errors for unopenable files; wrapped with %w.
var (
	ErrNoEditor   = errors.New("no terminal editor found (set $EDITOR or install micro, nano, or vi)")
	ErrNoTerminal = errors.New("no terminal emulator found (set $TERMINAL)")
)

// Runner executes one built command: name the terminal binary, args its
// argv tail. The platform default spawns the terminal detached.
type Runner func(name string, args ...string) error

// Opener builds terminal-editor commands for one GOOS and fires them via
// its Runner. Construct with New; the zero value is not usable.
type Opener struct {
	goos     string
	lookPath func(string) (string, error)
	getenv   func(string) string
	run      Runner
}

// New returns an Opener. Empty goos selects runtime.GOOS, nil lookPath
// selects exec.LookPath, nil getenv selects os.Getenv, nil r selects the
// platform detached-spawn runner.
func New(goos string, lookPath func(string) (string, error), getenv func(string) string, r Runner) *Opener {
	if goos == "" {
		goos = runtime.GOOS
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if r == nil {
		r = platformRunner()
	}
	return &Opener{goos: goos, lookPath: lookPath, getenv: getenv, run: r}
}

// Open opens path in a terminal editor inside a terminal emulator. The
// final spawned argv is [terminal, convention-args..., editor-argv...,
// path] with the ini path LAST. Terminal candidates are tried in order; a
// spawn failure falls through to the next one. A successful return means
// the spawn was requested — a detached spawn proves nothing about the
// editor actually running.
func (o *Opener) Open(path string) error {
	editor, err := o.editorArgv()
	if err != nil {
		return err
	}
	cands := o.terminalCandidates()
	if len(cands) == 0 {
		return ErrNoTerminal
	}
	var lastErr error
	for _, cand := range cands {
		argv := append(argvFor(cand, editor), path)
		log.Info().Str("terminal", argv[0]).Str("path", path).Msg("opening file in terminal editor")
		if err := o.run(argv[0], argv[1:]...); err != nil {
			log.Warn().Err(err).Str("terminal", argv[0]).Msg("terminal spawn failed, trying next")
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("termopen %q: every terminal emulator failed: %w", path, lastErr)
}

// editorArgv resolves the editor command: $EDITOR verbatim, split with
// strings.Fields (documented: no shell quoting — `EDITOR="code --wait"`
// works, quoted metacharacters do not); unset falls to the first of micro,
// nano, vi found via lookPath.
func (o *Opener) editorArgv() ([]string, error) {
	if ed := strings.TrimSpace(o.getenv("EDITOR")); ed != "" {
		return strings.Fields(ed), nil
	}
	for _, ed := range []string{"micro", "nano", "vi"} {
		if _, err := o.lookPath(ed); err == nil {
			return []string{ed}, nil
		}
	}
	return nil, ErrNoEditor
}

// argvFor assembles the spawn argv for one terminal candidate with the
// editor appended: [terminal, convention-args..., editor...]. This is the
// exact assembly Open uses per candidate, before appending the file path.
func argvFor(cand, editor []string) []string {
	return append(append([]string(nil), cand...), editor...)
}

// convention is how a terminal emulator accepts the command to run.
type convention int

const (
	convPositional convention = iota // argv appended verbatim: foot, kitty
	convDashE                        // -e prefix: konsole, alacritty, xterm
	convDashDash                     // -- prefix: gnome-terminal
)

// knownTerminals is the basename→convention table, in probe order.
var knownTerminals = []struct {
	name string
	conv convention
}{
	{"foot", convPositional},
	{"konsole", convDashE},
	{"gnome-terminal", convDashDash},
	{"kitty", convPositional},
	{"alacritty", convDashE},
	{"xterm", convDashE},
}

// terminalCandidates returns the terminal argv prefixes to try, in order:
// $TERMINAL first (known basename → its convention, unknown → -e
// best-effort), then every known-chain emulator found via lookPath.
func (o *Opener) terminalCandidates() [][]string {
	var cands [][]string
	seen := map[string]bool{}
	add := func(name string, conv convention) {
		if seen[name] {
			return
		}
		seen[name] = true
		switch conv {
		case convDashE:
			cands = append(cands, []string{name, "-e"})
		case convDashDash:
			cands = append(cands, []string{name, "--"})
		default:
			cands = append(cands, []string{name})
		}
	}
	if term := strings.TrimSpace(o.getenv("TERMINAL")); term != "" {
		conv := convDashE // unknown basename: best-effort -e
		for _, kt := range knownTerminals {
			if kt.name == filepath.Base(term) {
				conv = kt.conv
				break
			}
		}
		add(term, conv)
	}
	for _, kt := range knownTerminals {
		if _, err := o.lookPath(kt.name); err == nil {
			add(kt.name, kt.conv)
		}
	}
	return cands
}
