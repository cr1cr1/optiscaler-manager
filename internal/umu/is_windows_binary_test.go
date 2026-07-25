package umu

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsWindowsBinary_TrueForExeExtension(t *testing.T) {
	if !IsWindowsBinary("/games/Foo/foo.exe") {
		t.Errorf("foo.exe: got false, want true")
	}
}

func TestIsWindowsBinary_TrueForBatAndCmdExtensions(t *testing.T) {
	for _, ext := range []string{".bat", ".cmd", ".msi"} {
		path := "/games/Foo/installer" + ext
		if !IsWindowsBinary(path) {
			t.Errorf("%s: got false, want true", path)
		}
	}
}

func TestIsWindowsBinary_FalseForLinuxExtensions(t *testing.T) {
	for _, ext := range []string{".sh", ".bin", ".appimage", ""} {
		path := "/games/Foo/launch" + ext
		if IsWindowsBinary(path) {
			t.Errorf("%s: got true, want false", path)
		}
	}
}

func TestIsWindowsBinary_TrueForPEMagicBytes(t *testing.T) {
	// File with no .exe extension but PE MZ header is still a Windows
	// binary (rare but happens with renamed installers).
	dir := t.TempDir()
	path := filepath.Join(dir, "game-noext")
	if err := os.WriteFile(path, []byte{'M', 'Z', 0x90, 0x00, 'f', 'o', 'o'}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !IsWindowsBinary(path) {
		t.Errorf("PE MZ header file: got false, want true")
	}
}

func TestIsWindowsBinary_FalseForNonPEBinary(t *testing.T) {
	// ELF file with no Windows extension.
	dir := t.TempDir()
	path := filepath.Join(dir, "game-noext")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if IsWindowsBinary(path) {
		t.Errorf("ELF file: got true, want false")
	}
}

func TestIsWindowsBinary_FalseForMissingFileButExeExt(t *testing.T) {
	// The .exe extension alone is enough; missing file is treated as
	// "yes, assume Windows binary" because the user explicitly named it
	// .exe. We don't want to refuse launching a game just because the
	// stat failed.
	if !IsWindowsBinary("/nonexistent/path/game.exe") {
		t.Errorf("missing .exe: got false, want true (extension wins)")
	}
}

func TestIsWindowsBinary_FalseForMissingFileNoExeExt(t *testing.T) {
	if IsWindowsBinary("/nonexistent/path/game") {
		t.Errorf("missing no-ext file: got true, want false")
	}
}
