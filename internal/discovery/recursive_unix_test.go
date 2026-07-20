//go:build !windows && !darwin

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeRawFile writes content with the given mode, bypassing the exe
// candidacy helpers so tests control magic bytes and exec bits exactly.
func writeRawFile(t *testing.T, path string, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// TestFileCandidate_RequiresBinaryMagic: on unix the old rule (any execute
// bit, or a .exe suffix) let arbitrary files become the "game executable" —
// the user's steamapps row pointed at a -rwxr-xr-x shader cache (.foz).
// Candidacy now requires real binary magic: MZ (Windows PE) or ELF.
func TestFileCandidate_RequiresBinaryMagic(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		mode    os.FileMode
		want    bool
	}{
		{"run.sh", "#!/bin/sh\nexit 0\n", 0o755, false},               // script, not a binary
		{"cache.foz", "fozpipelines-binary-blob", 0o755, false},       // shader cache with exec bit
		{"appmanifest.acf", "\"AppState\" {}", 0o755, false},          // steam metadata with exec bit
		{"notes.txt", "hello", 0o755, false},                          // text with exec bit
		{"game", "\x7fELF\x02\x01\x01\x00 padded", 0o755, true},       // extensionless ELF
		{"game.x86_64", "\x7fELF\x02\x01\x01\x00 godot", 0o755, true}, // Godot/Unity ELF
		{"game.exe", "MZ\x90\x00 fake-pe", 0o644, true},               // PE without exec bit
		{"fake.exe", "just text renamed", 0o755, false},               // .exe suffix but not a PE
		{"lib.dll", "MZ\x90\x00 dll", 0o755, false},                   // DLLs stay excluded
		{"lib.so", "\x7fELF\x02\x01\x01\x00 shared", 0o755, false},    // shared libs stay excluded
	}
	want := map[string]bool{}
	for _, tc := range cases {
		writeRawFile(t, filepath.Join(dir, tc.name), tc.content, tc.mode)
		want[tc.name] = tc.want
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		ok, _ := fileCandidate(filepath.Join(dir, e.Name()), e)
		if ok != want[e.Name()] {
			t.Errorf("fileCandidate(%s) = %v, want %v", e.Name(), ok, want[e.Name()])
		}
	}
}

// TestScanRecursive_ShadercacheJunkIsNotAGame reproduces the user's
// phantom "steamapps" row: a steamapps dir whose only "executable" is a
// shader-cache blob with the exec bit set must not classify as a game;
// the real game under common still surfaces.
func TestScanRecursive_ShadercacheJunkIsNotAGame(t *testing.T) {
	root := t.TempDir()
	writeRawFile(t, filepath.Join(root, "steamapps", "shadercache", "553850", "fozpipelinesv6", "steam_pipeline_cache.foz"), "foz-blob", 0o755)
	writeRawFile(t, filepath.Join(root, "steamapps", "appmanifest_553850.acf"), "\"AppState\" {}", 0o755)
	writePEExe(t, filepath.Join(root, "steamapps"), "common/RealGame/RealGame.exe", "Real Game Title")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d rows, want exactly 1 (RealGame): %v", len(games), games)
	}
	wantDir := canonicalPath(filepath.Join(root, "steamapps", "common", "RealGame"))
	if games[0].Name != "Real Game Title" || games[0].InstallDir != wantDir {
		t.Errorf("row = %q installDir=%q, want %q at %q", games[0].Name, games[0].InstallDir, "Real Game Title", wantDir)
	}
}
