//go:build !linux

package umu

import "context"

// RunnerRoots is empty off-Linux: Proton runners are a Linux concept
// and the launch path that consumes them is Linux-only.
func RunnerRoots() []string { return nil }

// FindRunners finds nothing off-Linux (see RunnerRoots).
func FindRunners(_ context.Context, _ []string) []Runner { return nil }
