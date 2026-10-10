//go:build linux

package launchercli

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/launchlog"
)

// Launch spawns the owning launcher's CLI detached (own session, Start
// + Release — never Wait, mirroring internal/launch's game spawner).
// Errors name the missing CLI and the remedy; a successful return only
// proves the CLI was started.
//
// With opts.LogDir set, the child's stdout/stderr are captured to a
// per-launch file (issue 061) headed by the exact argv — the file is
// where silent launcher failures get diagnosed. The file is closed
// right after Start; the child holds its own inherited descriptors.
func Launch(_ context.Context, opts Opts) error {
	argv, err := Command(opts.Source, opts.Ref, opts.Exe, opts.Flatpak)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("launchercli: %s not on PATH — install it or set Launch via: umu", cliBinary(opts.Source, opts.Flatpak))
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	logPath := ""
	if opts.LogDir != "" {
		p, f, err := launchlog.Create(opts.LogDir, opts.LogName)
		if err != nil {
			// Logging must never block a launch.
			log.Warn().Err(err).Str("dir", opts.LogDir).Msg("launch log unavailable")
		} else {
			defer f.Close()
			logPath = p
			launchlog.WriteHeader(f, argv, nil)
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}

	if err := cmd.Start(); err != nil {
		err = fmt.Errorf("launchercli: start %s: %w", argv[0], err)
		if logPath != "" {
			err = fmt.Errorf("%w\nfull log: %s", err, logPath)
		}
		return err
	}
	return cmd.Process.Release()
}
