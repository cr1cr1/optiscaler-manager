package pever

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bodgit/sevenzip"
)

// extractEntry extracts the first archive entry whose base name matches
// base (case-insensitive) into dir and returns its path ("" if absent).
func extractEntry(t *testing.T, zr *sevenzip.ReadCloser, base, dir string) string {
	t.Helper()
	for _, f := range zr.File {
		if !strings.EqualFold(filepath.Base(f.Name), base) || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %q: %v", f.Name, err)
		}
		dst := filepath.Join(dir, base)
		out, err := os.Create(dst)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			t.Fatalf("extract %q: %v", f.Name, err)
		}
		_ = out.Close()
		_ = rc.Close()
		return dst
	}
	return ""
}

// TestPEFileVersion_RealOptiScaler094 is a surface check against the real
// OptiScaler 0.9.4 release archive: every shipped upscaler DLL must get
// the correct version AND marketing name from the reference-vendored
// tables. It SKIPS unless OM_TEST_ARCHIVE points at the bundle .7z.
//
// Expected values were determined by running against the real bundle
// (fixed FILEVERSION quad / ProductVersion string noted per row):
//
//	DLL                                    raw           source          marketing
//	OptiScaler.dll                         0.9.4         product         (no kind)
//	amd_fidelityfx_dx12.dll                2.3.0.0       product*        FSR 4.1
//	amd_fidelityfx_framegeneration_dx12.dll 4.0.1.0      product         FSR 4.1
//	amd_fidelityfx_upscaler_dx12.dll       4.1.1.0       product         FSR 4.1
//	amd_fidelityfx_vk.dll                  1.0.1.41314   fixed (product placeholder 1.0.1.0)  FSR 3.1.4
//	libxess.dll                            2.0.2.68      fixed/product   XeSS 2.0
//
// *amd_fidelityfx_dx12.dll is the FidelityFX dispatch shim; the reference
// map has no 2.3.x key, so the tier-2 same-major fallback below
// 2.2.0.1328 labels it FSR 4.1 — consistent with the bundle's upscaler
// DLL (4.1.1.0). The framegeneration DLL (4.0.1.0) likewise falls to
// tier-4 nearest-below FSR 4.1. The 0.9.4 bundle ships no nvngx_dlss.dll;
// if a future bundle does, the DLSS row below catches table drift.
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

	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".dll") {
			t.Logf("archive DLL entry: %s (%d bytes)", f.Name, f.FileInfo().Size())
		}
	}

	dir := t.TempDir()

	t.Run("OptiScaler.dll version", func(t *testing.T) {
		p := extractEntry(t, zr, "OptiScaler.dll", dir)
		if p == "" {
			t.Fatal("OptiScaler.dll not found in archive")
		}
		got, err := FileVersion(p)
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		if got != "0.9.4" {
			t.Errorf("got %q, want %q", got, "0.9.4")
		}
	})

	rows := []struct {
		base string
		kind Kind
		raw  string
		want string
	}{
		{"amd_fidelityfx_vk.dll", KindFSR, "1.0.1.41314", "FSR 3.1.4"},
		{"amd_fidelityfx_dx12.dll", KindFSR, "2.3.0.0", "FSR 4.1"},
		{"amd_fidelityfx_upscaler_dx12.dll", KindFSR, "4.1.1.0", "FSR 4.1"},
		{"amd_fidelityfx_framegeneration_dx12.dll", KindFSR, "4.0.1.0", "FSR 4.1"},
		{"libxess.dll", KindXeSS, "2.0.2.68", "XeSS 2.0"},
	}
	for _, row := range rows {
		t.Run(row.base, func(t *testing.T) {
			p := extractEntry(t, zr, row.base, dir)
			if p == "" {
				t.Fatalf("%s not found in archive", row.base)
			}
			raw, err := FileVersion(p)
			if err != nil {
				t.Fatalf("FileVersion: %v", err)
			}
			if raw != row.raw {
				t.Errorf("FileVersion = %q, want %q (bundle changed? re-derive expectations)", raw, row.raw)
			}
			if got := MarketingName(row.kind, raw); got != row.want {
				t.Errorf("MarketingName = %q, want %q", got, row.want)
			}
			t.Logf("%s: version %s → %s", row.base, raw, MarketingName(row.kind, raw))
		})
	}

	t.Run("nvngx_dlss.dll if shipped", func(t *testing.T) {
		p := extractEntry(t, zr, "nvngx_dlss.dll", dir)
		if p == "" {
			t.Skip("0.9.4 ships no nvngx_dlss.dll")
		}
		raw, err := FileVersion(p)
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		got := MarketingName(KindDLSS, raw)
		t.Logf("nvngx_dlss.dll: version %s → %s", raw, got)
		if got == raw {
			t.Errorf("nvngx_dlss.dll %q has no marketing mapping; DLSS table drifted", raw)
		}
	})
}
