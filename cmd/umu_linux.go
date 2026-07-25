//go:build linux

package optiscalermanager

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// newUmuLauncher returns the ui-layer hook that drives umu-run for
// umu-eligible manual-store games. Returns nil when umu-run is not
// installed or its --version output is unparseable, in which case the
// session transparently falls back to the regular Launcher.
//
// Detection runs once at session construction; we do not re-query
// umu-run on every launch (cheap as it is, it's a fork+exec and a
// small UX win is not worth the latency on a click that should feel
// instant).
func newUmuLauncher(prefs settings.Settings) ui.UmuLauncherHook {
	path, version, err := umu.Detect(context.Background())
	if err != nil {
		log.Info().Err(err).Msg("umu-launcher not detected; Windows-binary launches will fall back to the regular Launcher")
		return nil
	}
	log.Info().Str("path", path).Str("version", version).Msg("umu-launcher detected")
	return func(ctx context.Context, row ui.GameRow) error {
		prefix, err := umu.PrefixFor(row.InstallDir, row.Title)
		if err != nil {
			return err
		}
		opts := umu.LaunchOpts{
			ExePath:    row.ExePath,
			GameID:     "umu-default",
			WinePrefix: prefix,
			ProtonPath: prefs.UmuProtonPath,
			Store:      "",
			UmuRunPath: path,
		}
		return umu.Launch(ctx, nil, opts)
	}
}
