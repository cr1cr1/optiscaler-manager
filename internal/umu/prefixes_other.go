//go:build !linux

package umu

import "context"

// FindPrefixes finds nothing off-Linux: wine prefixes are a Linux
// concept and the launch path that consumes them is Linux-only.
func FindPrefixes(_ context.Context) []Prefix { return nil }

// LookupPrefix finds nothing off-Linux (see FindPrefixes).
func LookupPrefix(_ context.Context, _ string) (Prefix, bool) { return Prefix{}, false }
