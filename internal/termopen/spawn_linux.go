//go:build linux

package termopen

import (
	"os/exec"
	"syscall"
)

// platformRunner spawns the terminal detached (own session, Start +
// Release — never Wait). Stdin, stdout, and stderr are left nil: the child
// gets the null device.
func platformRunner() Runner {
	return func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}
