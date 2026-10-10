package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// Launch requests a game launch: it never blocks the caller. Routed
// Windows binaries (umu / owner CLI) run supervised — the slot marks
// the game running and CancelOp stops it (issue 068); other targets
// stay fire-and-forget ("Launch requested", never "launched").
func (s *Session) Launch(gameDir string) {
	go s.doLaunch(gameDir)
}

// registerLaunch marks a routed launch as running (false when one
// already runs — the double-launch guard).
func (s *Session) registerLaunch(gameDir string, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, running := s.launchRuns[gameDir]; running {
		return false
	}
	s.launchRuns[gameDir] = cancel
	return true
}

// finishLaunch releases the launch slot for gameDir.
func (s *Session) finishLaunch(gameDir string) {
	s.mu.Lock()
	delete(s.launchRuns, gameDir)
	s.mu.Unlock()
}

// LaunchBusy reports whether a routed launch for gameDir is running
// (the umu backend's lifetime is the game's). Frontends gate Stop
// affordances on it.
func (s *Session) LaunchBusy(gameDir string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, running := s.launchRuns[gameDir]
	return running
}

func (s *Session) doLaunch(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil {
		s.launchFailed(fmt.Errorf("unknown game %s", gameDir), gameDir)
		return
	}
	target, err := s.launchTarget(row)
	if err != nil {
		s.launchFailed(err, gameDir)
		return
	}

	// Windows-binary short-circuit: when a router hook is wired
	// (production: always on Linux) AND the target is umu-eligible
	// (Linux + umu store + Windows binary), bypass the regular
	// Launcher entirely — raw PE exec lands in the kernel's binfmt
	// (mono) and hangs. The hook routes to the prefix owner's CLI or
	// umu-run per the route setting (issue 060) and owns env-var setup
	// and stderr-scanning for umu's exit-0-on-fatal-error quirk.
	//
	// Supervision (issue 068): the launch registers a context for its
	// whole lifetime — the umu backend blocks until the game exits, so
	// the slot marks the game running, guards against double-launch,
	// and CancelOp stops the process group ("Stopped"). Owner-CLI
	// backends return at spawn; their stop goes through deps.UmuStop.
	if s.shouldRouteWinBinary(row) {
		ctx, cancel := context.WithCancel(context.Background())
		if !s.registerLaunch(gameDir, cancel) {
			s.toast(row.Title+" is already running", true)
			return
		}
		defer s.finishLaunch(gameDir)
		s.setStatusGame(gameDir, "Running: "+row.Title)
		via, err := s.deps.UmuLauncher(ctx, *row)
		var what string
		switch {
		case ctx.Err() != nil:
			what = "Stopped: " + row.Title
		case err != nil:
			s.launchFailed(err, gameDir)
			return
		case via == "umu-launcher":
			// The blocking backend returning nil means the game exited
			// on its own.
			what = row.Title + " exited"
		default:
			what = "Launch requested: " + row.Title + " (via " + via + ")"
		}
		s.setStatusGame(gameDir, what)
		s.toast(what, false)
		s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
		return
	}

	if err := s.deps.Launcher.Launch(context.Background(), target); err != nil {
		s.launchFailed(err, gameDir)
		return
	}
	what := "Launch requested: " + row.Title
	s.setStatusGame(gameDir, what)
	s.toast(what, false)
	s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
}

// UmuPlatform reports whether the umu-launcher integration applies on
// this platform (Linux only): the frontends gate the prefix pickers on
// it so Windows/macOS users never see wine-prefix controls.
func (s *Session) UmuPlatform() bool {
	goos := s.deps.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	return goos == "linux"
}

// shouldRouteWinBinary reports whether the Windows-binary launch
// router takes this launch. The checks are: a hook is wired, we're on
// Linux, the row is a game the regular Launcher can only run through
// Windows-store verbs (manual, or Heroic-discovered Epic/GOG — their
// Linux store URLs are best-effort at best), and its ExePath is a
// Windows binary (PE MZ header or .exe/.bat/.cmd/.msi extension).
// UmuEnabled is intentionally NOT a gate: it only controls the umu
// backend inside the router (issue 060).
func (s *Session) shouldRouteWinBinary(row *GameRow) bool {
	if s.deps.UmuLauncher == nil {
		return false
	}
	if !s.UmuPlatform() {
		return false
	}
	switch row.Store {
	case domain.StoreManual, domain.StoreEpic, domain.StoreGOG:
	default:
		return false
	}
	return umu.IsWindowsBinary(row.ExePath)
}

func (s *Session) launchFailed(err error, gameDir string) {
	s.setStatusGame(gameDir, "Launch failed: "+err.Error())
	s.toast("Launch failed: "+err.Error(), true)
	s.emit(Event{Kind: EvOpFailed, Text: err.Error(), GameDir: gameDir})
}

// launchTarget maps a row to its launch target; manual games launch through
// the user's template, and a blank ExePath on an exe-launched store falls
// back to the discovery exe picking before giving up.
func (s *Session) launchTarget(row *GameRow) (launch.Target, error) {
	t := launch.Target{
		Store:   launchStore(row.Store),
		Name:    row.Title,
		AppID:   row.AppID,
		AppName: row.AppName,
		ExePath: row.ExePath,
		Dir:     row.InstallDir,
	}
	if t.Store == launch.StoreManual {
		t.Template = s.Settings().LaunchTemplate
	}
	if t.ExePath == "" && (t.Store == launch.StoreManual || t.Store == launch.StoreGOG) {
		exe, err := resolveGameExe(row.InstallDir)
		if err != nil {
			return t, err
		}
		t.ExePath = exe
	}
	return t, nil
}

func launchStore(s domain.Store) launch.Store {
	switch s {
	case domain.StoreEpic:
		return launch.StoreEpic
	case domain.StoreGOG:
		return launch.StoreGOG
	case domain.StoreManual:
		return launch.StoreManual
	case domain.StoreSteam:
		return launch.StoreSteam
	default:
		return launch.StoreUnknown
	}
}

// resolveGameExe reuses the recursive scanner's exe picking for one game
// directory whose ExePath discovery left blank. When the parent scan
// yields nothing (e.g. the parent dir is engine-named and refused as a
// scan root), the game's own directory is searched directly.
func resolveGameExe(gameDir string) (string, error) {
	games, err := discovery.ScanRecursive(context.Background(), filepath.Dir(gameDir))
	if err != nil {
		return "", fmt.Errorf("resolve exe for %s: %w", gameDir, err)
	}
	want := canonicalDir(gameDir)
	for _, g := range games {
		if g.InstallDir == want && g.ExePath != "" {
			return g.ExePath, nil
		}
	}
	if exe, err := discovery.FindMainExe(context.Background(), gameDir); err == nil && exe != "" {
		return exe, nil
	}
	return "", fmt.Errorf("no executable found under %s", gameDir)
}
