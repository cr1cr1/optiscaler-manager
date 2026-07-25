package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// launchCapture records the argv handed to the injected runner.
type launchCapture struct {
	mu    sync.Mutex
	calls int
	dir   string
	name  string
	args  []string
	err   error
}

func (c *launchCapture) runner() launch.Runner {
	return func(_ context.Context, dir, name string, args ...string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls++
		c.dir, c.name, c.args = dir, name, append([]string(nil), args...)
		return c.err
	}
}

func (c *launchCapture) argv() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls == 0 {
		return nil
	}
	return append([]string{c.name}, c.args...)
}

func noBinaries(string) (string, error) { return "", errors.New("not found") }

func addRow(s *Session, r GameRow) {
	s.mu.Lock()
	s.st.Rows = append(s.st.Rows, r)
	s.mu.Unlock()
}

func TestSessionLaunchSteamBuildsRunGameID(t *testing.T) {
	e := newTestEnv(t)
	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	dir := "/games/steam/GameOne"
	addRow(e.sess, GameRow{Title: "Game One", AppID: "100", InstallDir: dir, Store: domain.StoreSteam})

	e.sess.Launch(dir)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.HasPrefix(ev.Text, "Launch requested:") {
		t.Errorf("event text %q, want \"Launch requested: …\" prefix", ev.Text)
	}

	argv := cap.argv()
	found := false
	for _, a := range argv {
		if strings.Contains(a, "steam://rungameid/100") {
			found = true
		}
	}
	if !found {
		t.Fatalf("captured argv %v lacks steam://rungameid/100", argv)
	}
	t.Logf("captured argv: %v", argv)

	st := e.sess.Snapshot()
	if len(st.Toasts) == 0 || !strings.HasPrefix(st.Toasts[len(st.Toasts)-1].Text, "Launch requested: Game One") {
		t.Errorf("toasts %+v, want trailing \"Launch requested: Game One\"", st.Toasts)
	}
	for _, toast := range st.Toasts {
		low := strings.ToLower(toast.Text)
		if strings.Contains(low, "launched") || strings.Contains(low, "running") {
			t.Errorf("toast %q overstates spawn success (never say launched/running)", toast.Text)
		}
	}
}

func TestSessionLaunchManualUsesTemplate(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	exe := "/games/manual/MyGame/mygame.exe"
	dir := "/games/manual/MyGame"
	addRow(e.sess, GameRow{Title: "My Game", InstallDir: dir, Store: domain.StoreManual, ExePath: exe})

	e.sess.SetLaunchTemplate(`umu-run "{exe}" -- {args}`)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	argv := cap.argv()
	if len(argv) < 3 || argv[0] != "umu-run" || argv[1] != exe || argv[2] != "--" {
		t.Fatalf("captured argv %v, want [umu-run %s -- …] from custom template", argv, exe)
	}
	t.Logf("captured argv: %v", argv)
}

func TestSessionLaunchUnknownStoreErrors(t *testing.T) {
	e := newTestEnv(t)
	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	dir := "/games/nowhere/Mystery"
	addRow(e.sess, GameRow{Title: "Mystery", InstallDir: dir, Store: domain.Store(42)})

	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpFailed)
	if cap.calls != 0 {
		t.Fatalf("runner called %d times for an unknown store, want 0", cap.calls)
	}
	st := e.sess.Snapshot()
	if len(st.Toasts) == 0 || !strings.HasPrefix(st.Toasts[len(st.Toasts)-1].Text, "Launch failed:") {
		t.Errorf("toasts %+v, want trailing \"Launch failed: …\" warn toast", st.Toasts)
	} else if !st.Toasts[len(st.Toasts)-1].Warn {
		t.Error("launch-failure toast not marked warn")
	}
	t.Logf("unknown store: %q", st.Toasts[len(st.Toasts)-1].Text)
}

func TestSessionLaunchNotifiesOnSpawnFailure(t *testing.T) {
	e := newTestEnv(t)
	cap := &launchCapture{err: errors.New("spawn blew up")}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	dir := "/games/steam/GameOne"
	addRow(e.sess, GameRow{Title: "Game One", AppID: "100", InstallDir: dir, Store: domain.StoreSteam})

	e.sess.Launch(dir)
	ev := waitEvent(t, e.sess, EvOpFailed)
	if !strings.Contains(ev.Text, "spawn blew up") {
		t.Errorf("event text %q, want the runner error", ev.Text)
	}
	st := e.sess.Snapshot()
	last := st.Toasts[len(st.Toasts)-1]
	if !strings.Contains(last.Text, "Launch failed: spawn blew up") || !last.Warn {
		t.Errorf("toast %+v, want warn \"Launch failed: spawn blew up\"", last)
	}
	t.Logf("captured argv before failure: %v", cap.argv())
}

// umuCapture records the row handed to the injected UmuLauncher hook.
// Mirrors launchCapture for the umu-launcher path.
type umuCapture struct {
	mu    sync.Mutex
	calls int
	row   GameRow
	err   error
}

func (u *umuCapture) hook() UmuLauncherHook {
	return func(_ context.Context, row GameRow) error {
		u.mu.Lock()
		defer u.mu.Unlock()
		u.calls++
		u.row = row
		return u.err
	}
}

// TestSessionLaunch_ManualWindowsGameUsesUmuWhenEnabled proves the umu
// hook takes precedence over the regular Launcher when settings allow it
// and the target is umu-eligible (manual store + Windows binary).
func TestSessionLaunch_ManualWindowsGameUsesUmuWhenEnabled(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	// Create an actual .exe on disk so IsWindowsBinary has something to
	// stat (extension alone suffices, but the on-disk route exercises
	// the full eligibility check).
	dir := "/games/manual/WinGame"
	exe := dir + "/game.exe"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: exe})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 1 {
		t.Fatalf("UmuLauncher.calls = %d, want 1", ucap.calls)
	}
	if cap.calls != 0 {
		t.Errorf("regular Launcher.calls = %d, want 0 (umu hook should win)", cap.calls)
	}
	if ucap.row.ExePath != exe {
		t.Errorf("UmuLauncher called with ExePath %q, want %q", ucap.row.ExePath, exe)
	}
}

// TestSessionLaunch_ManualWindowsGameFallsBackWhenUmuDisabled: the
// setting is the gate; default off means the regular Launcher runs.
func TestSessionLaunch_ManualWindowsGameFallsBackWhenUmuDisabled(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/manual/WinGame"
	exe := dir + "/game.exe"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: exe})

	// UmuEnabled stays at the default (false).
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 0 {
		t.Errorf("UmuLauncher.calls = %d, want 0 (feature disabled)", ucap.calls)
	}
	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1 (fallback)", cap.calls)
	}
}

// TestSessionLaunch_ManualWindowsGameFallsBackWhenNoUmuLauncher: the
// hook being nil (production default on non-Linux or when umu-run isn't
// installed) means regular Launcher runs even if UmuEnabled is on.
func TestSessionLaunch_ManualWindowsGameFallsBackWhenNoUmuLauncher(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	e.sess.deps.UmuLauncher = nil // no umu-run detected

	dir := "/games/manual/WinGame"
	exe := dir + "/game.exe"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: exe})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1 when UmuLauncher is nil", cap.calls)
	}
}

// TestSessionLaunch_ManualLinuxBinaryIgnoresUmu: a manual game with a
// shell-script launcher is not umu-eligible; the regular Launcher runs.
func TestSessionLaunch_ManualLinuxBinaryIgnoresUmu(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/manual/NativeGame"
	addRow(e.sess, GameRow{Title: "Native", InstallDir: dir, Store: domain.StoreManual, ExePath: dir + "/start.sh"})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 0 {
		t.Errorf("UmuLauncher.calls = %d, want 0 (.sh is not a Windows binary)", ucap.calls)
	}
	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1", cap.calls)
	}
}

// TestSessionLaunch_SteamGameNeverUsesUmu: non-manual stores bypass
// umu entirely; Steam always goes through the regular steam:// launcher.
func TestSessionLaunch_SteamGameNeverUsesUmu(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/steam/SteamGame"
	addRow(e.sess, GameRow{Title: "Steam Win", AppID: "999", InstallDir: dir, Store: domain.StoreSteam, ExePath: dir + "/game.exe"})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 0 {
		t.Errorf("UmuLauncher.calls = %d, want 0 (Steam never uses umu)", ucap.calls)
	}
	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1", cap.calls)
	}
}

// TestSessionLaunch_UmuFailureReportedAsLaunchFailed: umu.Launch errors
// surface as EvOpFailed + warn toast, exactly like regular launch errors.
func TestSessionLaunch_UmuFailureReportedAsLaunchFailed(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{err: errors.Join(umu.ErrUmuSetupFailed, errors.New("FileNotFoundError: PROTONPATH"))}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/manual/WinGame"
	exe := dir + "/game.exe"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: exe})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	ev := waitEvent(t, e.sess, EvOpFailed)
	if !strings.Contains(ev.Text, "FileNotFoundError") {
		t.Errorf("event text %q, want the umu error", ev.Text)
	}
	if cap.calls != 0 {
		t.Errorf("regular Launcher.calls = %d, want 0 (umu hook returned an error, no fallback)", cap.calls)
	}
	st := e.sess.Snapshot()
	last := st.Toasts[len(st.Toasts)-1]
	if !last.Warn || !strings.Contains(last.Text, "Launch failed:") {
		t.Errorf("toast %+v, want warn \"Launch failed: …\"", last)
	}
}
