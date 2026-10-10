//go:build linux

package umu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckProton: empty means umu's own default (always valid);
// a pinned path must be a runner dir with an executable proton
// script (issue 065).
func TestCheckProton(t *testing.T) {
	if err := CheckProton(""); err != nil {
		t.Errorf("empty proton path: %v, want nil (umu default)", err)
	}

	runner := t.TempDir()
	if err := CheckProton(runner); err == nil || !strings.Contains(err.Error(), "proton") {
		t.Errorf("dir without proton script: %v, want error naming proton", err)
	}

	script := filepath.Join(runner, "proton")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckProton(runner); err == nil {
		t.Error("non-executable proton script: want error, got nil")
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckProton(runner); err != nil {
		t.Errorf("valid runner: %v, want nil", err)
	}

	if err := CheckProton(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("missing runner dir: want error, got nil")
	}
}

// TestCheckPrefixWritable: the prefix (or its deepest existing
// ancestor for not-yet-created app-managed prefixes) must be writable
// — umu creates lock files at the prefix root and fails deep inside
// Python otherwise (issue 065).
func TestCheckPrefixWritable(t *testing.T) {
	existing := t.TempDir()
	if err := CheckPrefixWritable(existing); err != nil {
		t.Errorf("writable dir: %v, want nil", err)
	}

	pending := filepath.Join(existing, "umu-prefixes", "abc123")
	if err := CheckPrefixWritable(pending); err != nil {
		t.Errorf("creatable prefix under writable parent: %v, want nil", err)
	}

	ro := filepath.Join(existing, "readonly")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrefixWritable(ro); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Errorf("read-only prefix: %v, want error naming not writable", err)
	}
	if err := CheckPrefixWritable(filepath.Join(ro, "child")); err == nil {
		t.Error("prefix under read-only parent: want error, got nil")
	}
}

// TestCheckExe: the game exe must exist — a stale ExePath otherwise
// fails inside wine with a generic error (issue 065).
func TestCheckExe(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "game.exe")
	if err := CheckExe(exe); err == nil || !strings.Contains(err.Error(), "game.exe") {
		t.Errorf("missing exe: %v, want error naming the exe", err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckExe(exe); err != nil {
		t.Errorf("present exe: %v, want nil", err)
	}
}
