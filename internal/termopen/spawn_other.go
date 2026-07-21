//go:build !linux

package termopen

import "errors"

// platformRunner is a stub: terminal-editor opening is only wired on linux;
// darwin and windows keep their own openExternal branches.
func platformRunner() Runner {
	return func(name string, args ...string) error {
		return errors.New("termopen: terminal editor opening is only supported on linux")
	}
}
