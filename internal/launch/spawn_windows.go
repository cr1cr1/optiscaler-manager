//go:build windows

package launch

import (
	"context"
	"os/exec"
	"syscall"
)

// Windows process-creation flags (winbase.h), defined locally — syscall
// does not export them.
const (
	createNewProcessGroup = 0x00000200 // CREATE_NEW_PROCESS_GROUP
	detachedProcess       = 0x00000008 // DETACHED_PROCESS
)

// platformRunner spawns games detached (no console, own process group,
// Start + Release — never Wait) and waits on URL openers so their failures
// surface. Stdin, stdout, and stderr are left nil.
func platformRunner() Runner {
	return func(ctx context.Context, dir, name string, args ...string) error {
		if isURLOpener(name) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = dir
			return cmd.Run()
		}
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: createNewProcessGroup | detachedProcess,
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}
