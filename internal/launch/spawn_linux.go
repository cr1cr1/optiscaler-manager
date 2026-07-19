//go:build linux

package launch

import (
	"context"
	"os/exec"
	"syscall"
)

// platformRunner spawns games detached (own session, Start + Release —
// never Wait) and waits on URL openers so their failures surface. Stdin,
// stdout, and stderr are left nil: the child gets the null device.
func platformRunner() Runner {
	return func(ctx context.Context, dir, name string, args ...string) error {
		if isURLOpener(name) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = dir
			return cmd.Run()
		}
		// Detached: no CommandContext — ctx cancellation must not kill the game.
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}
