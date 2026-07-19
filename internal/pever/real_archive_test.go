package pever

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bodgit/sevenzip"
)

// TestPEFileVersion_RealOptiScaler094 is a manual surface check against the
// real OptiScaler 0.9.4 release archive. It SKIPS unless OM_TEST_ARCHIVE
// points at the bundle .7z.
func TestPEFileVersion_RealOptiScaler094(t *testing.T) {
	archivePath := os.Getenv("OM_TEST_ARCHIVE")
	if archivePath == "" {
		t.Skip("OM_TEST_ARCHIVE unset; skipping real-archive check")
	}

	zr, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = zr.Close() }()

	var dllPath string
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), "OptiScaler.dll") && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open entry %q: %v", f.Name, err)
			}
			dllPath = filepath.Join(t.TempDir(), "OptiScaler.dll")
			out, err := os.Create(dllPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(out, rc); err != nil {
				t.Fatalf("extract: %v", err)
			}
			_ = out.Close()
			_ = rc.Close()
			break
		}
	}
	if dllPath == "" {
		t.Fatalf("OptiScaler.dll not found in %s", archivePath)
	}

	got, err := FileVersion(dllPath)
	if err != nil {
		t.Fatalf("FileVersion: %v", err)
	}
	t.Logf("OptiScaler.dll version: %q", got)
	if got != "0.9.4" {
		t.Errorf("got %q, want %q", got, "0.9.4")
	}
}
