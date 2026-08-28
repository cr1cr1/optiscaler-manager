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

// Launch requests a fire-and-forget game launch: it never blocks, never
// waits on the child, and a successful request proves nothing about the
// game actually running — hence "Launch requested", never "launched".
func (s *Session) Launch(gameDir string) {
	go s.doLaunch(gameDir)
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

	// umu-launcher short-circuit: when the feature is enabled AND an
	// umu hook is wired (production: umu-run detected) AND the target
	// is umu-eligible (Linux + manual store + Windows binary), bypass
	// the regular Launcher entirely. The hook owns env-var setup and
	// stderr-scanning for umu's exit-0-on-fatal-error quirk.
	if s.shouldUseUmu(row) {
		if err := s.deps.UmuLauncher(context.Background(), *row); err != nil {
			s.launchFailed(err, gameDir)
			return
		}
		what := "Launch requested: " + row.Title + " (via umu-launcher)"
		s.setStatus(what)
		s.toast(what, false)
		s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
		return
	}

	if err := s.deps.Launcher.Launch(context.Background(), target); err != nil {
		s.launchFailed(err, gameDir)
		return
	}
	what := "Launch requested: " + row.Title
	s.setStatus(what)
	s.toast(what, false)
	s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
}

// shouldUseUmu reports whether the umu-launcher path should be taken
// for this launch. The checks are: the setting is on, a hook is wired,
// we're on Linux, the row is a manual-store game, and its ExePath is a
// Windows binary (PE MZ header or .exe/.bat/.cmd/.msi extension).
func (s *Session) shouldUseUmu(row *GameRow) bool {
	if s.deps.UmuLauncher == nil {
		return false
	}
	goos := s.deps.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "linux" {
		return false
	}
	if !s.Settings().UmuEnabled {
		return false
	}
	if row.Store != domain.StoreManual {
		return false
	}
	return umu.IsWindowsBinary(row.ExePath)
}

func (s *Session) launchFailed(err error, gameDir string) {
	s.setStatus("Launch failed: " + err.Error())
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
