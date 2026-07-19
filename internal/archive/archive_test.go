package archive

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestSevenzipExtractsRealOptiScaler094Archive is the M2d spike gate: it runs
// only against a real OptiScaler 0.9.4 bundle (set OM_TEST_ARCHIVE to its
// path) and proves the pure-Go decoder — including the BCJ2 filter used on
// x86 DLLs — handles the exact artifact the installer will consume.
//
// Run with: OM_TEST_ARCHIVE=/path/to/Optiscaler_0.9.4-final.*.7z go test ./internal/archive/
func TestSevenzipExtractsRealOptiScaler094Archive(t *testing.T) {
	path := os.Getenv("OM_TEST_ARCHIVE")
	if path == "" {
		t.Skip("OM_TEST_ARCHIVE not set; spike gate requires a real Optiscaler 0.9.4 .7z")
	}

	names, err := List(path)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("archive has no entries")
	}
	t.Logf("archive has %d entries", len(names))
	for _, n := range names {
		t.Logf("  entry: %s", n)
	}

	// Required-file set (verified against the real 0.9.4 bundle: OptiScaler.dll
	// + fakenvapi.dll/fakenvapi.ini + NukemFG + FFX/XeSS SDKs, mostly at
	// archive root plus D3D12_Optiscaler/ and Licenses/).
	base, err := EntryNames(path)
	if err != nil {
		t.Fatalf("EntryNames: %v", err)
	}
	for _, want := range []string{"optiscaler.dll", "fakenvapi.dll", "fakenvapi.ini", "dlssg_to_fsr3_amd_is_better.dll"} {
		if !slices.Contains(base, want) {
			t.Errorf("required file %q missing from bundle", want)
		}
	}

	// Find the OptiScaler.dll entry (any directory depth) and hash it fully;
	// a BCJ2 decode failure surfaces here, not at List time.
	var dllEntry string
	for _, n := range names {
		if filepath.Base(n) == "OptiScaler.dll" {
			dllEntry = n
			break
		}
	}
	if dllEntry == "" {
		t.Fatal("OptiScaler.dll entry not found")
	}
	digest, size, err := HashEntry(path, dllEntry)
	if err != nil {
		t.Fatalf("HashEntry(%q): %v", dllEntry, err)
	}
	if size == 0 {
		t.Fatal("OptiScaler.dll decompressed to zero bytes")
	}
	t.Logf("OptiScaler.dll: %d bytes, sha256 %s", size, digest)

	// Full extraction into staging must succeed with sanitization active.
	dst := t.TempDir()
	if err := ExtractTo(path, dst); err != nil {
		t.Fatalf("ExtractTo: %v", err)
	}
	rel, err := SanitizeName(dllEntry)
	if err != nil {
		t.Fatalf("SanitizeName(%q): %v", dllEntry, err)
	}
	st, err := os.Stat(filepath.Join(dst, rel))
	if err != nil {
		t.Fatalf("extracted OptiScaler.dll missing: %v", err)
	}
	if st.Size() != size {
		t.Fatalf("extracted size %d != hashed size %d", st.Size(), size)
	}
	t.Logf("full extraction into %s succeeded", dst)
}

func TestSanitizeNameRejectsUnsafe(t *testing.T) {
	bad := []string{
		"", "/etc/passwd", "//server/share/x", "C:\\windows\\system32",
		"../evil", "a/../../evil", "..\\evil", "a/./../../evil",
	}
	for _, name := range bad {
		if _, err := SanitizeName(name); err == nil {
			t.Errorf("SanitizeName(%q): expected rejection", name)
		} else {
			t.Logf("rejected %q: %v", name, err)
		}
	}

	good := map[string]string{
		"OptiScaler.dll":        "OptiScaler.dll",
		"amd64/nvapi64.dll":     filepath.Join("amd64", "nvapi64.dll"),
		"a\\b\\c.dll":           filepath.Join("a", "b", "c.dll"),
		"dir with spaces/f.ini": filepath.Join("dir with spaces", "f.ini"),
	}
	for in, want := range good {
		got, err := SanitizeName(in)
		if err != nil {
			t.Errorf("SanitizeName(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
