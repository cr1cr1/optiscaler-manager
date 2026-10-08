//go:build linux

package optiscalermanager

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// newUmuLauncher returns the ui-layer hook that drives umu-run for
// umu-eligible manual and Heroic Epic/GOG games. Returns nil when umu-run is not
// installed or its --version output is unparseable, in which case the
// session transparently falls back to the regular Launcher.
//
// pinPrefix resolves the effective pinned prefix for an install dir at
// launch time (session.UmuPrefixFor: per-game pin, else the global
// default, else "" for the auto chain). It is read per launch so pins
// changed mid-session apply without a restart. Proton is always Auto:
// the newest runner detected across the Steam / Bottles / Lutris /
// Heroic / umu roots (umu's own runtime when nothing is found).
//
// Detection runs once at session construction; we do not re-query
// umu-run on every launch (cheap as it is, it's a fork+exec and a
// small UX win is not worth the latency on a click that should feel
// instant).
func newUmuLauncher(pinPrefix func(installDir string) string) ui.UmuLauncherHook {
	path, version, err := umu.Detect(context.Background())
	if err != nil {
		log.Info().Err(err).Msg("umu-launcher not detected; Windows-binary launches will fall back to the regular Launcher")
		return nil
	}
	log.Info().Str("path", path).Str("version", version).Msg("umu-launcher detected")
	return func(ctx context.Context, row ui.GameRow) error {
		prefix, err := umu.ResolvePrefix(pinPrefix(row.InstallDir), row.InstallDir, row.ExePath, row.AppName)
		if err != nil {
			return err
		}
		proton := umu.ResolveProtonPath(ctx, "")
		log.Debug().Str("game", row.Title).Str("proton", proton).Str("prefix", prefix).
			Msg("umu launch resolved")
		opts := umu.LaunchOpts{
			ExePath:    row.ExePath,
			GameID:     "umu-default",
			WinePrefix: prefix,
			ProtonPath: proton,
			Store:      "",
			UmuRunPath: path,
		}
		if err := umu.Launch(ctx, nil, opts); err != nil {
			// The error already carries umu's stderr tail; the fields
			// name what was attempted so a failure in the console is
			// traceable to a game, binary, prefix, and runner.
			log.Error().Err(err).
				Str("game", row.Title).
				Str("exe", row.ExePath).
				Str("prefix", prefix).
				Str("proton", proton).
				Msg("umu launch failed")
			return err
		}
		return nil
	}
}
