//go:build linux

package launchercli

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

// Launch spawns the owning launcher's CLI detached (own session, Start
// + Release — never Wait, mirroring internal/launch's game spawner).
// Errors name the missing CLI and the remedy; a successful return only
// proves the CLI was started.
func Launch(_ context.Context, source, ref, exe string, flatpak bool) error {
	argv, err := Command(source, ref, exe, flatpak)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("launchercli: %s not on PATH — install it or set Launch via: umu", cliBinary(source, flatpak))
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launchercli: start %s: %w", argv[0], err)
	}
	return cmd.Process.Release()
}
