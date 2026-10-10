package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
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
// Mirrors launchCapture for the umu-launcher path. via is what the
// router reports as the chosen backend ("umu-launcher", "bottles", …).
type umuCapture struct {
	mu    sync.Mutex
	calls int
	row   GameRow
	via   string
	err   error
}

func (u *umuCapture) hook() UmuLauncherHook {
	return func(_ context.Context, row GameRow) (string, error) {
		u.mu.Lock()
		defer u.mu.Unlock()
		u.calls++
		u.row = row
		via := u.via
		if via == "" {
			via = "umu-launcher"
		}
		return via, u.err
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

// TestSessionLaunch_ManualWindowsGameRoutesEvenWhenUmuDisabled: the
// router hook owns the launch decision for Windows binaries (issue
// 060); UmuEnabled only gates the umu backend INSIDE the router. Raw
// PE exec through the regular Launcher never happens on Linux (it
// lands in the kernel's binfmt, e.g. mono, and hangs).
func TestSessionLaunch_ManualWindowsGameRoutesEvenWhenUmuDisabled(t *testing.T) {
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

	if ucap.calls != 1 {
		t.Errorf("UmuLauncher.calls = %d, want 1 (router owns PE launches)", ucap.calls)
	}
	if cap.calls != 0 {
		t.Errorf("regular Launcher.calls = %d, want 0 (never raw-exec a PE)", cap.calls)
	}
}

// TestSessionLaunch_ReportsChosenBackend: the router's via string
// names the backend in the status/toast (issue 060).
func TestSessionLaunch_ReportsChosenBackend(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()
	ucap := &umuCapture{via: "bottles"}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/manual/WinGame"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: dir + "/game.exe"})

	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)
	st := e.sess.Snapshot().StatusLine
	if !strings.Contains(st, "via bottles") {
		t.Errorf("status %q lacks the chosen backend", st)
	}
}

// TestSessionLaunchSupervision: a routed launch registers a running
// slot for its whole lifetime — LaunchBusy reports it, a second
// Launch is refused, CancelOp stops it ("Stopped", no Failed), the
// slot releases, and a later clean exit reports "exited" (issue 068).
func TestSessionLaunchSupervision(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	var mu sync.Mutex
	calls := 0
	started := make(chan struct{}, 2)
	unblock := make(chan struct{})
	e.sess.deps.UmuLauncher = func(ctx context.Context, row GameRow) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-unblock:
			return "umu-launcher", nil
		}
	}

	dir := "/games/manual/WinGame"
	addRow(e.sess, GameRow{Title: "Win Game", InstallDir: dir, Store: domain.StoreManual, ExePath: dir + "/game.exe"})

	e.sess.Launch(dir)
	<-started
	if !e.sess.LaunchBusy(dir) {
		t.Fatal("LaunchBusy = false while the game runs")
	}

	e.sess.Launch(dir) // double-launch: refused
	mu.Lock()
	if calls != 1 {
		t.Errorf("hook calls = %d after double-launch, want 1", calls)
	}
	mu.Unlock()
	pollUITill(t, "double-launch toast", func() bool {
		for _, to := range e.sess.Snapshot().Toasts {
			if strings.Contains(to.Text, "already running") {
				return true
			}
		}
		return false
	})

	if !e.sess.CancelOp(dir) {
		t.Fatal("CancelOp = false, want the launch stopped")
	}
	waitEvent(t, e.sess, EvOpDone)
	pollUITill(t, "stopped status", func() bool {
		return strings.Contains(e.sess.Snapshot().StatusLine, "Stopped")
	})
	if e.sess.LaunchBusy(dir) {
		t.Error("LaunchBusy still true after stop — the slot must release")
	}
	for _, to := range e.sess.Snapshot().Toasts {
		if strings.HasPrefix(to.Text, "Failed:") {
			t.Errorf("stop produced a Failed toast: %+v", to)
		}
	}

	// Relaunch after stop; a clean exit reports "exited".
	close(unblock)
	e.sess.Launch(dir)
	<-started
	waitEvent(t, e.sess, EvOpDone)
	pollUITill(t, "exited status", func() bool {
		return strings.Contains(e.sess.Snapshot().StatusLine, "exited")
	})
}

// TestSessionLaunchStopOwnerCLI: with no op and no umu launch slot,
// CancelOp falls through to the deps stopper (owner-CLI launches,
// issue 068).
func TestSessionLaunchStopOwnerCLI(t *testing.T) {
	e := newTestEnv(t)
	stopped := map[string]bool{}
	e.sess.deps.UmuStop = func(dir string) bool { return stopped[dir] }
	dir := "/games/manual/WinGame"

	if e.sess.CancelOp(dir) {
		t.Error("CancelOp = true with nothing running")
	}
	stopped[dir] = true
	if !e.sess.CancelOp(dir) {
		t.Error("CancelOp = false, want the deps stopper consulted")
	}
}

// TestSessionLaunchRoute: the global LaunchRoute default persists, a
// per-game override wins, clearing restores the default, and an empty
// value means Auto (issue 060).
func TestSessionLaunchRoute(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.Scan(t.Context())
	waitScanDone(t, e.sess)
	dir := e.gameRoot

	e.sess.SetLaunchRoute("owner")
	if got := e.sess.Settings().LaunchRoute; got != "owner" {
		t.Errorf("LaunchRoute = %q, want owner", got)
	}
	if got := e.sess.LaunchRouteFor(dir); got != "owner" {
		t.Errorf("LaunchRouteFor = %q, want owner", got)
	}

	e.sess.SetLaunchRouteOverride(dir, "umu")
	if got := e.sess.LaunchRoutePin(dir); got != "umu" {
		t.Errorf("LaunchRoutePin = %q, want umu", got)
	}
	if got := e.sess.LaunchRouteFor(dir); got != "umu" {
		t.Errorf("LaunchRouteFor = %q, want per-game umu", got)
	}
	s, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.LaunchRoute != "owner" || s.LaunchRouteOverrides[canonicalDir(dir)] != "umu" {
		t.Errorf("persisted = %+v, want owner + per-game umu", s)
	}

	e.sess.SetLaunchRouteOverride(dir, "")
	if got := e.sess.LaunchRouteFor(dir); got != "owner" {
		t.Errorf("LaunchRouteFor after clear = %q, want owner", got)
	}
	e.sess.SetLaunchRoute("")
	if got := e.sess.LaunchRouteFor(dir); got != "" {
		t.Errorf("LaunchRouteFor = %q, want empty (Auto)", got)
	}

	e.sess.SetLaunchRouteOverride("/no/such/game", "umu")
	if len(e.sess.Settings().LaunchRouteOverrides) != 0 {
		t.Errorf("override persisted for unknown game: %v", e.sess.Settings().LaunchRouteOverrides)
	}
}

// TestSessionUmuRunner: the global UmuRunnerPath default persists, a
// per-game override wins, clearing restores the default, and an empty
// value means Auto (newest installed) — the LaunchRoute precedent
// (issue 063).
func TestSessionUmuRunner(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.Scan(t.Context())
	waitScanDone(t, e.sess)
	dir := e.gameRoot

	e.sess.SetUmuRunnerPath("/runners/global-proton")
	if got := e.sess.Settings().UmuRunnerPath; got != "/runners/global-proton" {
		t.Errorf("UmuRunnerPath = %q, want /runners/global-proton", got)
	}
	if got := e.sess.UmuRunnerFor(dir); got != "/runners/global-proton" {
		t.Errorf("UmuRunnerFor = %q, want the global default", got)
	}

	e.sess.SetUmuRunnerOverride(dir, "/runners/per-game-proton")
	if got := e.sess.UmuRunnerPin(dir); got != "/runners/per-game-proton" {
		t.Errorf("UmuRunnerPin = %q, want /runners/per-game-proton", got)
	}
	if got := e.sess.UmuRunnerFor(dir); got != "/runners/per-game-proton" {
		t.Errorf("UmuRunnerFor = %q, want per-game pin", got)
	}
	s, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.UmuRunnerPath != "/runners/global-proton" || s.RunnerOverrides[canonicalDir(dir)] != "/runners/per-game-proton" {
		t.Errorf("persisted = %+v, want global + per-game runner", s)
	}

	e.sess.SetUmuRunnerOverride(dir, "")
	if got := e.sess.UmuRunnerFor(dir); got != "/runners/global-proton" {
		t.Errorf("UmuRunnerFor after clear = %q, want the global default", got)
	}
	e.sess.SetUmuRunnerPath("")
	if got := e.sess.UmuRunnerFor(dir); got != "" {
		t.Errorf("UmuRunnerFor = %q, want empty (Auto)", got)
	}

	e.sess.SetUmuRunnerOverride("/no/such/game", "/runners/x")
	if len(e.sess.Settings().RunnerOverrides) != 0 {
		t.Errorf("override persisted for unknown game: %v", e.sess.Settings().RunnerOverrides)
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

// TestSessionLaunch_HeroicEpicGameUsesUmuWhenEnabled: a Heroic-discovered
// Epic row with an ExePath is umu-eligible on Linux — without it the
// launch would fall to the best-effort xdg-open Epic URL.
func TestSessionLaunch_HeroicEpicGameUsesUmuWhenEnabled(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/heroic/Fortnite"
	exe := dir + "/FortniteGame/Binaries/Win64/Fortnite.exe"
	addRow(e.sess, GameRow{Title: "Fortnite", InstallDir: dir, Store: domain.StoreEpic, AppName: "Fortnite", ExePath: exe})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 1 {
		t.Fatalf("UmuLauncher.calls = %d, want 1 (heroic epic row is umu-eligible)", ucap.calls)
	}
	if ucap.row.ExePath != exe {
		t.Errorf("UmuLauncher ExePath = %q, want %q", ucap.row.ExePath, exe)
	}
}

// TestSessionLaunch_HeroicGOGGameUsesUmuWhenEnabled: same for GOG rows —
// without umu the regular Launcher would try to exec a Windows PE
// directly, which fails on Linux.
func TestSessionLaunch_HeroicGOGGameUsesUmuWhenEnabled(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := "/games/heroic/Hades"
	exe := dir + "/Hades.exe"
	addRow(e.sess, GameRow{Title: "Hades", InstallDir: dir, Store: domain.StoreGOG, ExePath: exe})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 1 {
		t.Fatalf("UmuLauncher.calls = %d, want 1 (heroic gog row is umu-eligible)", ucap.calls)
	}
}

// TestSessionLaunch_HeroicRowWithoutExeStaysOnRegularLauncher: an
// Epic/GOG row with no resolved exe is not umu-eligible; the regular
// Launcher keeps its URL/direct-exe fallbacks.
func TestSessionLaunch_HeroicRowWithoutExeStaysOnRegularLauncher(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "linux"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	// Real dirs: the regular launcher resolves a blank ExePath by
	// scanning the install dir's PARENT, so the game lives one level
	// below the temp root to keep that scan small.
	root := t.TempDir()
	dir := filepath.Join(root, "NoExe")
	exe := filepath.Join(dir, "game.exe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ\x90\x00"), 0o755); err != nil { // 4+ bytes: isBinaryMagic ReadAt()s 4
		t.Fatal(err)
	}
	addRow(e.sess, GameRow{Title: "No Exe Recorded", InstallDir: dir, Store: domain.StoreGOG})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 0 {
		t.Errorf("UmuLauncher.calls = %d, want 0 (no exe)", ucap.calls)
	}
	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1", cap.calls)
	}
}

// TestSessionLaunch_HeroicRowIgnoresUmuOnWindows: natively on Windows the
// Epic/GOG stores launch through their own verbs; umu is Linux-only.
func TestSessionLaunch_HeroicRowIgnoresUmuOnWindows(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.GOOS = "windows"
	e.sess.deps.SettingsRoot = t.TempDir()

	cap := &launchCapture{}
	e.sess.deps.Launcher = launch.New(cap.runner(), "windows", noBinaries)
	ucap := &umuCapture{}
	e.sess.deps.UmuLauncher = ucap.hook()

	dir := `C:\Games\Hades`
	addRow(e.sess, GameRow{Title: "Hades", InstallDir: dir, Store: domain.StoreGOG, ExePath: dir + `\Hades.exe`})

	e.sess.SetUmuEnabled(true)
	e.sess.Launch(dir)
	waitEvent(t, e.sess, EvOpDone)

	if ucap.calls != 0 {
		t.Errorf("UmuLauncher.calls = %d, want 0 (windows)", ucap.calls)
	}
	if cap.calls != 1 {
		t.Errorf("regular Launcher.calls = %d, want 1", cap.calls)
	}
}
