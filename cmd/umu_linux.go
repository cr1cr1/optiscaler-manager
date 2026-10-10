//go:build linux

package optiscalermanager

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/launchercli"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// umuNetProbe checks network reachability for the runtime-download
// preflight (issue 065). A package var so tests can stub it.
var umuNetProbe = func(ctx context.Context) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", "umu.openwinecomponents.org:443")
	if err != nil {
		return err
	}
	return conn.Close()
}

// newUmuLauncher returns the ui-layer hook that routes Windows-binary
// launches for umu-eligible manual and Heroic Epic/GOG games (issue
// 060). Despite the name it is the launch ROUTER: per the game's
// effective route (per-game override → global default → auto) it
// launches via the prefix owner's CLI (bottles-cli, lutris, heroic) or
// via umu-run, and never falls through to the regular Launcher — raw
// PE exec lands in the kernel's binfmt (mono) and hangs.
//
// pinPrefix resolves the effective pinned prefix for an install dir at
// launch time (session.UmuPrefixFor); routeFor resolves the route
// (session.LaunchRouteFor). Both are read per launch so changes
// mid-session apply without a restart.
//
// auto semantics: a launcher-owned prefix (per the prefix scan)
// launches via the owner's CLI — an unroutable owner is a launch ERROR
// naming the missing piece, never a silent umu fallback. An unowned
// prefix (app-managed, ~/.wine, unknown) launches via umu-run; with
// umu-run not installed that is an error naming the remedy. The runner
// still follows the prefix's launcher (issue 059) for umu launches.
//
// umu detection runs once at construction; the hook is always wired on
// Linux, with or without umu-run, so owner routing works with the umu
// feature toggle off (the toggle only gates the umu backend).
//
// logDir enables per-launch log files (issue 061) for both backends —
// empty disables. Production wires <data root>/logs.
//
// runnerFor resolves the effective pinned Proton build for an install
// dir at launch time (session.UmuRunnerFor, issue 063): "" means Auto
// (newest detected matching the prefix's launcher, else global).
//
// gameIDFor resolves the GAMEID/STORE pair at launch time
// (session.UmuGameIDFor, issue 064) so protonfixes apply per-game
// fixes; it degrades to ("umu-default", "") when unresolvable.
func newUmuLauncher(pinPrefix, routeFor func(installDir string) string, umuAllowed func() bool, logDir string, runnerFor func(installDir string) string, gameIDFor func(ctx context.Context, row ui.GameRow) (string, string)) ui.UmuLauncherHook {
	umuPath, umuVersion, umuErr := umu.Detect(context.Background())
	if umuErr != nil {
		log.Info().Err(umuErr).Msg("umu-run not detected; unowned Windows-binary launches will error unless the prefix owner routes elsewhere")
	} else {
		log.Info().Str("path", umuPath).Str("version", umuVersion).Msg("umu-launcher detected")
	}
	// Busy prefixes (issue 066): the first launch into a prefix uses
	// waitforexitandrun; while a game we launched still runs there,
	// further launches use runinprefix — waiting would block until the
	// first game's wineserver exits. umu.Launch blocks until exit, so
	// the registry entry's lifetime matches the game's.
	var verbMu sync.Mutex
	busy := map[string]int{}
	return func(ctx context.Context, row ui.GameRow) (string, error) {
		route := routeFor(row.InstallDir)
		prefix, err := umu.ResolvePrefix(pinPrefix(row.InstallDir), row.InstallDir, row.ExePath, row.AppName)
		if err != nil {
			return "", err
		}
		pfx, owned := umu.LookupPrefix(ctx, prefix)

		launchUmu := func() (string, error) {
			if !umuAllowed() {
				return "", fmt.Errorf("umu-launcher is disabled — enable it in Settings, or pin a launcher-owned prefix (Bottles/Lutris/Heroic)")
			}
			if umuErr != nil {
				return "", fmt.Errorf("umu-run unavailable (%v) — install umu-launcher or pin a launcher-owned prefix (Bottles/Lutris/Heroic)", umuErr)
			}
			// Guard (issue 062): umu mutates any Proton prefix it
			// launches (DLL symlinks, pfx/ layout), so it must never
			// run against a prefix OWNED BY ANOTHER LAUNCHER — that
			// corrupts the owner's bottle/compatdata (observed: Bottles
			// prefixes needed repair). ~/.wine and unrecognized dirs
			// have no launcher owner and stay umu-eligible. Auto never
			// reaches this path with an owned prefix (issue 060 routes
			// those to the owner CLI); only a forced umu route can.
			if owned && pfx.Source != "wine" {
				return "", fmt.Errorf("prefix %s is managed by %s — umu-run would corrupt it; pin an app-managed/umu prefix or set Launch via: auto/owner", prefix, pfx.Source)
			}
			// Preflight (issue 065): fail fast naming the broken piece
			// instead of dying inside umu-run's Python — invalid
			// runner, unwritable prefix, stale exe, or a first-run
			// runtime download with no network.
			proton := umu.ResolveProtonPath(ctx, runnerFor(row.InstallDir), pfx.Source)
			if err := umu.CheckProton(proton); err != nil {
				return "", err
			}
			if err := umu.CheckPrefixWritable(prefix); err != nil {
				return "", err
			}
			if err := umu.CheckExe(row.ExePath); err != nil {
				return "", err
			}
			if !umu.RuntimePresent() {
				if err := umuNetProbe(ctx); err != nil {
					return "", fmt.Errorf("umu runtime not downloaded yet and network unreachable (%v) — first launch needs network or a pre-populated runtime", err)
				}
				log.Info().Str("game", row.Title).Msg("umu runtime not downloaded — first launch fetches it, expect a long run")
			}
			gameID, store := gameIDFor(ctx, row)
			verbMu.Lock()
			verb := "waitforexitandrun"
			if busy[prefix] > 0 {
				verb = "runinprefix"
			}
			busy[prefix]++
			verbMu.Unlock()
			defer func() {
				verbMu.Lock()
				busy[prefix]--
				if busy[prefix] <= 0 {
					delete(busy, prefix)
				}
				verbMu.Unlock()
			}()
			log.Debug().Str("game", row.Title).Str("proton", proton).Str("prefix", prefix).Str("prefix_source", pfx.Source).
				Str("route", route).Str("gameid", gameID).Str("store", store).Str("verb", verb).Msg("umu launch resolved")
			opts := umu.LaunchOpts{
				ExePath:    row.ExePath,
				GameID:     gameID,
				WinePrefix: prefix,
				ProtonPath: proton,
				Store:      store,
				UmuRunPath: umuPath,
				LogDir:     logDir,
				LogName:    row.Title,
				Verb:       verb,
			}
			if err := umu.Launch(ctx, nil, opts); err != nil {
				// The error already carries umu's stderr tail; the fields
				// name what was attempted so a failure in the console is
				// traceable to a game, binary, prefix, and runner.
				log.Error().Err(err).
					Str("game", row.Title).
					Str("exe", row.ExePath).
					Str("prefix", prefix).
					Str("prefix_source", pfx.Source).
					Str("proton", proton).
					Msg("umu launch failed")
				return "", err
			}
			return "umu-launcher", nil
		}

		launchOwner := func() (string, error) {
			if !owned {
				return "", fmt.Errorf("prefix %s has no owning launcher — pin a Bottles/Lutris/Heroic prefix or set Launch via: umu", prefix)
			}
			if err := launchercli.Launch(ctx, launchercli.Opts{
				Source: pfx.Source, Ref: pfx.OwnerRef, Exe: row.ExePath,
				Flatpak: pfx.Flatpak, LogDir: logDir, LogName: row.Title,
			}); err != nil {
				log.Error().Err(err).
					Str("game", row.Title).
					Str("exe", row.ExePath).
					Str("prefix", prefix).
					Str("owner", pfx.Source).
					Str("owner_ref", pfx.OwnerRef).
					Msg("owner launch failed")
				return "", err
			}
			log.Debug().Str("game", row.Title).Str("owner", pfx.Source).
				Str("owner_ref", pfx.OwnerRef).Str("prefix", prefix).
				Msg("launch routed to prefix owner")
			return pfx.Source, nil
		}

		switch route {
		case "umu":
			return launchUmu()
		case "owner":
			return launchOwner()
		default: // auto
			if owned {
				return launchOwner()
			}
			return launchUmu()
		}
	}
}
