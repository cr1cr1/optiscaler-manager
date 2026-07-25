//go:build linux

package umu

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// fakeLauncher is the test seam for the Launcher callback. It captures
// the *exec.Cmd that Launch built (so tests can inspect Env / Args /
// Path) and returns canned stdout/stderr/exit-code behaviour. Writing
// to cmd.Stderr mimics what a real umu-run subprocess would do.
type fakeLauncher struct {
	exitCode int
	stderr   string
	captured *exec.Cmd
}

func (f *fakeLauncher) run(_ context.Context, cmd *exec.Cmd) error {
	f.captured = cmd
	if f.stderr != "" && cmd.Stderr != nil {
		// Launch wires a *bytes.Buffer; WriteString is the public API.
		_, _ = writeString(cmd.Stderr, f.stderr)
	}
	if f.exitCode == 0 {
		return nil
	}
	return &fakeExitError{code: f.exitCode}
}

type fakeExitError struct{ code int }

func (e *fakeExitError) Error() string { return "exit status " + itoa(e.code) }
func (e *fakeExitError) ExitCode() int { return e.code }

// writeString writes s into w via reflection-free means. We need to
// support any io.Writer the real launch path might use (currently a
// *bytes.Buffer). Use fmt.Fprintln on the io.Writer iface instead.
func writeString(w interface{ Write([]byte) (int, error) }, s string) (int, error) {
	return w.Write([]byte(s))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var sign string
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return sign + string(digits)
}

func TestLaunch_SetsRequiredEnvAndArgv(t *testing.T) {
	rec := &fakeLauncher{}
	opts := LaunchOpts{
		ExePath:    "/games/Foo/foo.exe",
		Args:       []string{"-windowed", "-res=1920,1080"},
		GameID:     "umu-default",
		WinePrefix: "/home/me/.local/share/optiscaler-manager/umu-prefixes/abc123",
		ProtonPath: "/runners/GE-Proton9-3",
		UmuRunPath: "/usr/bin/umu-run",
		Store:      "",
	}
	if err := Launch(context.Background(), rec.run, opts); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	cmd := rec.captured
	if cmd.Path != "/usr/bin/umu-run" {
		t.Errorf("cmd.Path = %q, want /usr/bin/umu-run", cmd.Path)
	}
	wantArgv := []string{"/usr/bin/umu-run", "/games/Foo/foo.exe", "-windowed", "-res=1920,1080"}
	if !reflect.DeepEqual(cmd.Args, wantArgv) {
		t.Errorf("cmd.Args = %v, want %v", cmd.Args, wantArgv)
	}

	env := envMap(cmd.Env)
	required := map[string]string{
		"GAMEID":      "umu-default",
		"WINEPREFIX":  "/home/me/.local/share/optiscaler-manager/umu-prefixes/abc123",
		"PROTONPATH":  "/runners/GE-Proton9-3",
		"PROTON_VERB": "waitforexitandrun",
		"STORE":       "",
	}
	for k, want := range required {
		if got, ok := env[k]; !ok {
			t.Errorf("env missing required key %q", k)
		} else if got != want {
			t.Errorf("env[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestLaunch_OmitsProtonPathWhenEmpty(t *testing.T) {
	rec := &fakeLauncher{}
	opts := LaunchOpts{
		ExePath:    "/g/foo.exe",
		GameID:     "umu-default",
		WinePrefix: "/pfx",
		ProtonPath: "",
		UmuRunPath: "/usr/bin/umu-run",
	}
	if err := Launch(context.Background(), rec.run, opts); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, present := envMap(rec.captured.Env)["PROTONPATH"]; present {
		t.Errorf("env has PROTONPATH set when opts.ProtonPath was empty; umu default token should be used instead")
	}
}

func TestLaunch_GameIDDefaultsToUmuDefault(t *testing.T) {
	rec := &fakeLauncher{}
	opts := LaunchOpts{ExePath: "/g/foo.exe", WinePrefix: "/p", UmuRunPath: "/usr/bin/umu-run"}
	if err := Launch(context.Background(), rec.run, opts); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if got := envMap(rec.captured.Env)["GAMEID"]; got != "umu-default" {
		t.Errorf("GAMEID = %q, want umu-default when opts.GameID is empty", got)
	}
}

func TestLaunch_DetectsFatalErrorDespiteExitZero(t *testing.T) {
	// umu's __main__ catches BaseException and exits 0 after logging
	// a Python traceback. Launch must surface this as a real error.
	rec := &fakeLauncher{
		exitCode: 0,
		stderr:   "Traceback (most recent call last):\n  File ...umu_run.py...\nFileNotFoundError: Environment variable not set or is empty: PROTONPATH\n",
	}
	opts := LaunchOpts{ExePath: "/g/foo.exe", WinePrefix: "/p", UmuRunPath: "/usr/bin/umu-run"}
	err := Launch(context.Background(), rec.run, opts)
	if err == nil {
		t.Fatal("Launch returned nil error despite Python traceback on stderr with exit 0")
	}
	if !errors.Is(err, ErrUmuSetupFailed) {
		t.Errorf("err = %v, want errors.Is ErrUmuSetupFailed", err)
	}
}

func TestLaunch_FatalMarkersCoverKnownUmuFailures(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"python traceback", "Traceback (most recent call last):\n  ...", true},
		{"FileNotFoundError", "FileNotFoundError: PROTONPATH is not valid", true},
		{"runtime error", "RuntimeError: umu has not been setup for the user", true},
		{"value error", "ValueError: invalid WINEPREFIX", true},
		{"connection error (NOT fatal)", "ConnectionError: failed to download runtime", false},
		{"normal stderr noise", "info: starting up\ninfo: ready\n", false},
		{"empty stderr", "", false},
	}
	for _, c := range cases {
		got := isFatalUmuFailure(c.stderr)
		if got != c.want {
			t.Errorf("%s: isFatalUmuFailure(%q) = %v, want %v", c.name, c.stderr, got, c.want)
		}
	}
}

func TestLaunch_PassesThroughRealExitCodes(t *testing.T) {
	rec := &fakeLauncher{exitCode: 42, stderr: ""}
	opts := LaunchOpts{ExePath: "/g/foo.exe", WinePrefix: "/p", UmuRunPath: "/usr/bin/umu-run"}
	err := Launch(context.Background(), rec.run, opts)
	if err == nil {
		t.Fatal("Launch returned nil for non-zero exit")
	}
	if errors.Is(err, ErrUmuSetupFailed) {
		t.Errorf("err is ErrUmuSetupFailed; non-fatal exit codes must pass through unwrapped")
	}
	var exitErr *fakeExitError
	if !errors.As(err, &exitErr) || exitErr.code != 42 {
		t.Errorf("err = %v, want exit code 42", err)
	}
}

func TestLaunch_RejectsMissingExePath(t *testing.T) {
	rec := &fakeLauncher{}
	err := Launch(context.Background(), rec.run, LaunchOpts{UmuRunPath: "/usr/bin/umu-run"})
	if !errors.Is(err, ErrMissingExe) {
		t.Errorf("err = %v, want ErrMissingExe", err)
	}
	if rec.captured != nil {
		t.Errorf("runner was invoked despite missing ExePath")
	}
}

func TestLaunch_RejectsMissingUmuRunPath(t *testing.T) {
	rec := &fakeLauncher{}
	err := Launch(context.Background(), rec.run, LaunchOpts{ExePath: "/g/foo.exe"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if rec.captured != nil {
		t.Errorf("runner was invoked despite missing UmuRunPath")
	}
}

func TestLaunch_InjectsExtraEnv(t *testing.T) {
	rec := &fakeLauncher{}
	opts := LaunchOpts{
		ExePath: "/g/foo.exe", WinePrefix: "/p", UmuRunPath: "/usr/bin/umu-run",
		ExtraEnv: map[string]string{"UMU_LOG": "1", "DXVK_FRAME_LOG": "1"},
	}
	if err := Launch(context.Background(), rec.run, opts); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	env := envMap(rec.captured.Env)
	if env["UMU_LOG"] != "1" {
		t.Errorf("UMU_LOG = %q, want 1", env["UMU_LOG"])
	}
	if env["DXVK_FRAME_LOG"] != "1" {
		t.Errorf("DXVK_FRAME_LOG = %q, want 1", env["DXVK_FRAME_LOG"])
	}
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}
