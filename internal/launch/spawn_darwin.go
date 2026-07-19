//go:build darwin

package launch

import (
	"context"
	"os/exec"
)

// platformRunner spawns games detached (Start + Release — never Wait) and
// waits on URL openers so their failures surface. Stdin, stdout, and stderr
// are left nil.
func platformRunner() Runner {
	return func(ctx context.Context, dir, name string, args ...string) error {
		if isURLOpener(name) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = dir
			return cmd.Run()
		}
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}
