package optiscalermanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// hookState reports the hook file's park state: parked = only the
// .disabled name exists, active = only the real name exists.
func hookState(t *testing.T, gameRoot, hookName string) (parked, active bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(gameRoot, "bin", hookName+".disabled"))
	parked = err == nil
	_, err = os.Stat(filepath.Join(gameRoot, "bin", hookName))
	active = err == nil
	return parked, active
}

// brandHook overwrites the game's hook DLL with an OptiScaler-branded PE —
// the identity marker ActiveHook requires before it parks a file (the
// toggle must never park a lookalike like DXVK's dxgi.dll).
func brandHook(t *testing.T, gameRoot, hook string) {
	t.Helper()
	writeCmdTestFile(t, filepath.Join(gameRoot, "bin", hook),
		string(testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{})))
}

// TestHookCommandTogglesDisabled: --disable parks the hook DLL (rename,
// row flag flipped), --enable restores it — the CLI surface of the
// frontends' synchronous toggle.
func TestHookCommandTogglesDisabled(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	cmdQuickInstall(t, d, gameRoot)
	hook := "dxgi.dll"
	brandHook(t, gameRoot, hook)

	if parked, active := hookState(t, gameRoot, hook); !active || parked {
		t.Fatalf("fixture hook state parked=%v active=%v, want active", parked, active)
	}

	if err := (&HookCmd{Path: gameRoot, Disable: true}).Run(d); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if parked, active := hookState(t, gameRoot, hook); !parked || active {
		t.Errorf("after --disable: parked=%v active=%v, want parked", parked, active)
	}
	if got := out.String(); !strings.Contains(got, "disabled") {
		t.Errorf("disable output:\n%s", got)
	}

	out.Reset()
	if err := (&HookCmd{Path: gameRoot, Enable: true}).Run(d); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if parked, active := hookState(t, gameRoot, hook); parked || !active {
		t.Errorf("after --enable: parked=%v active=%v, want active", parked, active)
	}
	if got := out.String(); !strings.Contains(got, "enabled") {
		t.Errorf("enable output:\n%s", got)
	}
	t.Logf("hook transcript:\n%s", out.String())
}

// TestHookCommandAlreadyInStateIsNoOp: toggling to the state the hook is
// already in reports it and must NOT flip the state back.
func TestHookCommandAlreadyInStateIsNoOp(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	cmdQuickInstall(t, d, gameRoot)
	hook := "dxgi.dll"
	brandHook(t, gameRoot, hook)

	if err := (&HookCmd{Path: gameRoot, Disable: true}).Run(d); err != nil {
		t.Fatalf("disable leg: %v", err)
	}
	out.Reset()

	if err := (&HookCmd{Path: gameRoot, Disable: true}).Run(d); err != nil {
		t.Fatalf("redundant disable must succeed: %v", err)
	}
	if parked, active := hookState(t, gameRoot, hook); !parked || active {
		t.Errorf("redundant disable flipped the state back: parked=%v active=%v", parked, active)
	}
	if !strings.Contains(out.String(), "already disabled") {
		t.Errorf("redundant disable output:\n%s", out.String())
	}
}

// TestHookCommandUninstalledGameFails: the toggle is meaningless without
// an install; the command must fail (exit 1) instead of reporting a false
// "already enabled" success for the row's default Disabled=false.
func TestHookCommandUninstalledGameFails(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	err := (&HookCmd{Path: gameRoot, Disable: true}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
	if !strings.Contains(exit.Error(), "not installed") {
		t.Errorf("err = %v, want a not-installed explanation", exit.Error())
	}
}

// TestHookCommandUsageExitCode: `hook <path>` with neither state flag —
// or with both — is a kong PARSE error, usage exit code 2, through the
// real CLI entry point (the struct-level path is unreachable in
// production; parse failures happen before newDeps, so no fixture is
// needed and no machine state is touched).
func TestHookCommandUsageExitCode(t *testing.T) {
	for _, args := range [][]string{
		{"hook", "/nonexistent/game"},
		{"hook", "/nonexistent/game", "--enable", "--disable"},
	} {
		err := Run("test", args)
		exit, ok := err.(*ExitError)
		if !ok {
			t.Fatalf("%v: err = %v, want *ExitError", args, err)
		}
		if exit.Code != 2 {
			t.Errorf("%v: exit code = %d, want 2", args, exit.Code)
		}
		t.Logf("%v usage error: %v", args, err)
	}
}
