//go:build !linux

package umu

import "context"

// FindPrefixes finds nothing off-Linux: wine prefixes are a Linux
// concept and the launch path that consumes them is Linux-only.
func FindPrefixes(_ context.Context) []Prefix { return nil }

// PrefixSource finds nothing off-Linux (see FindPrefixes).
func PrefixSource(_ context.Context, _ string) string { return "" }
