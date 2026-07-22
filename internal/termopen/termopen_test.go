package termopen

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// envMap builds a getenv backed by a fixed map; unset keys return "".
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// lookPathSet resolves exactly the binaries in the set.
func lookPathSet(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/usr/bin/" + name, nil
		}
		return "", fmt.Errorf("%s: not found", name)
	}
}

// TestEditorArgvChain: $EDITOR wins verbatim (strings.Fields — documented:
// no shell quoting); unset falls to the first of micro, nano, vi found via
// lookPath; none found is an error.
func TestEditorArgvChain(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		lookPath func(string) (string, error)
		want     []string
		wantErr  error
	}{
		{
			name:     "EDITOR with args used verbatim",
			env:      map[string]string{"EDITOR": "code --wait"},
			lookPath: lookPathSet("micro"),
			want:     []string{"code", "--wait"},
		},
		{
			name:     "EDITOR single word",
			env:      map[string]string{"EDITOR": "emacs"},
			lookPath: lookPathSet(),
			want:     []string{"emacs"},
		},
		{
			name:     "unset prefers micro",
			env:      map[string]string{},
			lookPath: lookPathSet("micro", "nano", "vi"),
			want:     []string{"micro"},
		},
		{
			name:     "unset falls to nano",
			env:      map[string]string{},
			lookPath: lookPathSet("nano", "vi"),
			want:     []string{"nano"},
		},
		{
			name:     "unset falls to vi",
			env:      map[string]string{},
			lookPath: lookPathSet("vi"),
			want:     []string{"vi"},
		},
		{
			name:     "no editor anywhere is an error",
			env:      map[string]string{},
			lookPath: lookPathSet(),
			wantErr:  ErrNoEditor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := New("linux", tt.lookPath, envMap(tt.env), nil)
			got, err := o.editorArgv()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("editorArgv() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("editorArgv() unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("editorArgv() = %v, want %v", got, tt.want)
			}
			t.Logf("editor argv: %v", got)
		})
	}
}

// TestTerminalArgv: known $TERMINAL basenames use their run-a-command
// convention; unknown $TERMINAL gets the -e best-effort; unset probes the
// known chain foot→konsole→gnome-terminal→kitty→alacritty→xterm in order.
// Each want is the first terminalCandidates() prefix plus the editor,
// assembled by argvFor — the exact per-candidate assembly Open uses.
func TestTerminalArgv(t *testing.T) {
	editor := []string{"nano"}
	tests := []struct {
		name     string
		env      map[string]string
		lookPath func(string) (string, error)
		want     []string
		wantErr  error
	}{
		{
			name:     "foot takes positional argv",
			env:      map[string]string{"TERMINAL": "foot"},
			lookPath: lookPathSet(),
			want:     []string{"foot", "nano"},
		},
		{
			name:     "konsole takes -e",
			env:      map[string]string{"TERMINAL": "konsole"},
			lookPath: lookPathSet(),
			want:     []string{"konsole", "-e", "nano"},
		},
		{
			name:     "gnome-terminal takes --",
			env:      map[string]string{"TERMINAL": "gnome-terminal"},
			lookPath: lookPathSet(),
			want:     []string{"gnome-terminal", "--", "nano"},
		},
		{
			name:     "kitty takes positional argv",
			env:      map[string]string{"TERMINAL": "kitty"},
			lookPath: lookPathSet(),
			want:     []string{"kitty", "nano"},
		},
		{
			name:     "alacritty takes -e",
			env:      map[string]string{"TERMINAL": "alacritty"},
			lookPath: lookPathSet(),
			want:     []string{"alacritty", "-e", "nano"},
		},
		{
			name:     "xterm takes -e",
			env:      map[string]string{"TERMINAL": "xterm"},
			lookPath: lookPathSet(),
			want:     []string{"xterm", "-e", "nano"},
		},
		{
			name:     "basename of a path is matched",
			env:      map[string]string{"TERMINAL": "/usr/bin/kitty"},
			lookPath: lookPathSet(),
			want:     []string{"/usr/bin/kitty", "nano"},
		},
		{
			name:     "unknown TERMINAL gets -e best-effort",
			env:      map[string]string{"TERMINAL": "weirdterm"},
			lookPath: lookPathSet(),
			want:     []string{"weirdterm", "-e", "nano"},
		},
		{
			name:     "unset probes chain from foot",
			env:      map[string]string{},
			lookPath: lookPathSet("foot", "xterm"),
			want:     []string{"foot", "nano"},
		},
		{
			name:     "unset skips missing chain entries",
			env:      map[string]string{},
			lookPath: lookPathSet("kitty", "xterm"),
			want:     []string{"kitty", "nano"},
		},
		{
			name:     "unset with no emulator is an error",
			env:      map[string]string{},
			lookPath: lookPathSet(),
			wantErr:  ErrNoTerminal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := New("linux", tt.lookPath, envMap(tt.env), nil)
			cands := o.terminalCandidates()
			if tt.wantErr != nil {
				if len(cands) != 0 {
					t.Fatalf("terminalCandidates() = %v, want none (%v)", cands, tt.wantErr)
				}
				return
			}
			if len(cands) == 0 {
				t.Fatalf("terminalCandidates() empty, want first candidate %v", tt.want)
			}
			got := argvFor(cands[0], editor)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("terminalArgv() = %v, want %v", got, tt.want)
			}
			t.Logf("terminal argv: %v", got)
		})
	}
}

// TestTerminalFallbackOnSpawnFailure: an unknown $TERMINAL whose spawn fails
// does not abort the open — the next available emulator in the known chain
// is tried and spawned.
func TestTerminalFallbackOnSpawnFailure(t *testing.T) {
	env := envMap(map[string]string{"TERMINAL": "weirdterm", "EDITOR": "nano"})
	var calls [][]string
	r := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if name == "weirdterm" {
			return errors.New("spawn: weirdterm blew up")
		}
		return nil
	}
	o := New("linux", lookPathSet("kitty"), env, r)

	if err := o.Open("/game/OptiScaler.ini"); err != nil {
		t.Fatalf("Open() err = %v, want fallback to kitty", err)
	}
	if len(calls) != 2 {
		t.Fatalf("runner calls = %v, want weirdterm failure then kitty", calls)
	}
	want := []string{"kitty", "nano", "/game/OptiScaler.ini"}
	if !reflect.DeepEqual(calls[1], want) {
		t.Errorf("fallback call = %v, want %v", calls[1], want)
	}
	t.Logf("runner calls: %v", calls)
}

// TestOpenSpawnsDetached: Open fires the Runner exactly once on success with
// [terminal, convention-args..., editor-argv..., iniPath] — the ini path
// always LAST.
func TestOpenSpawnsDetached(t *testing.T) {
	env := envMap(map[string]string{"TERMINAL": "konsole", "EDITOR": "code --wait"})
	var calls [][]string
	r := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	o := New("linux", lookPathSet(), env, r)

	if err := o.Open("/game/OptiScaler.ini"); err != nil {
		t.Fatalf("Open() err = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("runner calls = %v, want exactly one", calls)
	}
	want := []string{"konsole", "-e", "code", "--wait", "/game/OptiScaler.ini"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Errorf("argv = %v, want %v", calls[0], want)
	}
	if got := calls[0][len(calls[0])-1]; got != "/game/OptiScaler.ini" {
		t.Errorf("last argv element = %q, want the ini path", got)
	}
	t.Logf("spawned argv: %v", calls[0])
}

// TestOpenNoEditor: editor resolution fails before any terminal is spawned.
func TestOpenNoEditor(t *testing.T) {
	called := false
	r := func(name string, args ...string) error {
		called = true
		return nil
	}
	o := New("linux", lookPathSet("foot"), envMap(map[string]string{}), r)
	err := o.Open("/game/OptiScaler.ini")
	if !errors.Is(err, ErrNoEditor) {
		t.Fatalf("Open() err = %v, want %v", err, ErrNoEditor)
	}
	if called {
		t.Error("runner invoked despite editor resolution failure")
	}
}

// TestOpenNoTerminal: with no $TERMINAL and no known emulator on PATH, Open
// fails with ErrNoTerminal before anything is spawned.
func TestOpenNoTerminal(t *testing.T) {
	called := false
	r := func(name string, args ...string) error {
		called = true
		return nil
	}
	o := New("linux", lookPathSet(), envMap(map[string]string{"EDITOR": "nano"}), r)
	err := o.Open("/game/OptiScaler.ini")
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("Open() err = %v, want %v", err, ErrNoTerminal)
	}
	if called {
		t.Error("runner invoked despite no terminal emulator being available")
	}
}

// TestOpenAllTerminalsFail: every candidate failing surfaces an error
// mentioning the last failure.
func TestOpenAllTerminalsFail(t *testing.T) {
	env := envMap(map[string]string{"EDITOR": "nano"})
	r := func(name string, args ...string) error {
		return fmt.Errorf("spawn %s: boom", name)
	}
	o := New("linux", lookPathSet("foot", "xterm"), env, r)
	err := o.Open("/game/OptiScaler.ini")
	if err == nil || !strings.Contains(err.Error(), "xterm") {
		t.Fatalf("Open() err = %v, want an error wrapping the last (xterm) failure", err)
	}
	t.Logf("all-terminals-fail error: %v", err)
}
