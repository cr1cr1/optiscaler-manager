package pever

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- minimal in-memory PE fixture builder -------------------------------

// buildPE constructs a minimal PE32 (pe32plus=false) or PE32+ image with a
// single .rsrc section containing resData. The resource data-directory
// entry points at the whole section payload.
func buildPE(pe32plus bool, resData []byte) []byte {
	const (
		eLfanew    = 0x40
		sectVA     = 0x1000
		sectRawOff = 0x200
	)
	optSize := 0xE0 // PE32: 96 + 16*8
	ddOff := 96
	magic := uint16(0x10B)
	if pe32plus {
		optSize = 0xF0 // PE32+: 112 + 16*8
		ddOff = 112
		magic = 0x20B
	}
	size := sectRawOff + len(resData)
	b := make([]byte, size)

	// DOS header.
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3C:], eLfanew)

	// PE signature + COFF header.
	copy(b[eLfanew:], "PE\x00\x00")
	coff := eLfanew + 4
	binary.LittleEndian.PutUint16(b[coff:], 0x14C)              // Machine
	binary.LittleEndian.PutUint16(b[coff+2:], 1)                // NumberOfSections
	binary.LittleEndian.PutUint16(b[coff+16:], uint16(optSize)) // SizeOfOptionalHeader

	// Optional header.
	opt := coff + 20
	binary.LittleEndian.PutUint16(b[opt:], magic)
	dd := opt + ddOff
	binary.LittleEndian.PutUint32(b[dd+2*8:], sectVA) // resource RVA
	binary.LittleEndian.PutUint32(b[dd+2*8+4:], uint32(len(resData)))

	// Section table (.rsrc).
	sec := opt + optSize
	copy(b[sec:], ".rsrc\x00\x00\x00")
	binary.LittleEndian.PutUint32(b[sec+8:], uint32(len(resData)))  // VirtualSize
	binary.LittleEndian.PutUint32(b[sec+12:], sectVA)               // VirtualAddress
	binary.LittleEndian.PutUint32(b[sec+16:], uint32(len(resData))) // SizeOfRawData
	binary.LittleEndian.PutUint32(b[sec+20:], sectRawOff)           // PointerToRawData

	copy(b[sectRawOff:], resData)
	return b
}

// fixedFileInfo returns a VS_FIXEDFILEINFO block for the dotted quad.
func fixedFileInfo(maj, min, patch, build uint16) []byte {
	b := make([]byte, 52)
	binary.LittleEndian.PutUint32(b[0:], 0xFEEF04BD)
	binary.LittleEndian.PutUint32(b[4:], 0x00010000) // dwStrucVersion
	binary.LittleEndian.PutUint32(b[8:], uint32(maj)<<16|uint32(min))
	binary.LittleEndian.PutUint32(b[12:], uint32(patch)<<16|uint32(build))
	return b
}

// utf16 encodes s as UTF-16LE bytes (no terminator).
func utf16le(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r > math.MaxUint16 {
			r = 0xFFFD
		}
		var tmp [2]byte
		binary.LittleEndian.PutUint16(tmp[:], uint16(r))
		out = append(out, tmp[:]...)
	}
	return out
}

// stringInfoString builds a minimal String structure for the StringFileInfo
// entry key = value, 4-byte aligned at off 0.
func stringInfoString(key, value string) []byte {
	kb := utf16le(key) // 2 bytes per char + 2 for NUL
	vb := utf16le(value)
	valWords := len(vb)/2 + 1
	// key+NUL occupies b[6:6+len(kb)+2]; the value starts at the next
	// 4-aligned offset.
	valOff := (6 + len(kb) + 2 + 3) &^ 3
	structLen := valOff + len(vb) + 2
	b := make([]byte, structLen)
	binary.LittleEndian.PutUint16(b[0:], uint16(structLen))
	binary.LittleEndian.PutUint16(b[2:], uint16(valWords))
	binary.LittleEndian.PutUint16(b[4:], 1) // wType = text
	copy(b[6:], kb)
	copy(b[valOff:], vb)
	return b
}

// productVersionString builds the StringFileInfo entry "ProductVersion".
func productVersionString(value string) []byte {
	return stringInfoString("ProductVersion", value)
}

// concatStringStructs concatenates String structures, zero-padding each to a
// 4-byte boundary as real StringFileInfo children are laid out.
func concatStringStructs(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out
}

func writeTempPE(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.dll")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- tests ---------------------------------------------------------------

func TestPEFileVersion_SyntheticFixture(t *testing.T) {
	t.Run("PE32 fixed info only", func(t *testing.T) {
		res := fixedFileInfo(1, 2, 3, 4)
		got, err := FileVersion(writeTempPE(t, buildPE(false, res)))
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		if got != "1.2.3.4" {
			t.Errorf("got %q, want %q", got, "1.2.3.4")
		}
	})

	t.Run("PE32+ fixed info only", func(t *testing.T) {
		res := fixedFileInfo(3, 7, 10, 0)
		got, err := FileVersion(writeTempPE(t, buildPE(true, res)))
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		if got != "3.7.10.0" {
			t.Errorf("got %q, want %q", got, "3.7.10.0")
		}
	})

	t.Run("product version wins over placeholder fixed info", func(t *testing.T) {
		res := append(fixedFileInfo(1, 0, 0, 0), productVersionString("0.9.4")...)
		got, err := FileVersion(writeTempPE(t, buildPE(true, res)))
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		if got != "0.9.4" {
			t.Errorf("got %q, want %q", got, "0.9.4")
		}
	})

	t.Run("fixed info wins when product version is placeholder", func(t *testing.T) {
		res := append(fixedFileInfo(2, 4, 12, 0), productVersionString("1.0.0.0")...)
		got, err := FileVersion(writeTempPE(t, buildPE(false, res)))
		if err != nil {
			t.Fatalf("FileVersion: %v", err)
		}
		if got != "2.4.12.0" {
			t.Errorf("got %q, want %q", got, "2.4.12.0")
		}
	})
}

func TestPEFileVersion_RejectsTruncated(t *testing.T) {
	full := buildPE(true, fixedFileInfo(1, 2, 3, 4))

	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"empty", []byte{}, ErrNotPE},
		{"no MZ", []byte(strings.Repeat("NO", 64)), ErrNotPE},
		{"truncated DOS header", full[:0x20], ErrNotPE},
		{"missing PE signature", full[:0x50], ErrNotPE},
		{"bad PE signature", withBadSig(full), ErrNotPE},
		{"truncated COFF header", full[:0x40+4+10], ErrNotPE},
		{"truncated optional header", full[:0x40+4+20+0x40], ErrNotPE},
		{"bad optional magic", withBadOptMagic(full), ErrNotPE},
		{"truncated section table", full[:0x40+4+20+0xF0+10], ErrNotPE},
		{"no resource data", buildPE(false, nil), ErrNoVersionInfo},
		{"resource without version info", buildPE(false, []byte("deadbeefdeadbeef")), ErrNoVersionInfo},
		{"wrong struc version", buildPE(false, badStrucVersion()), ErrNoVersionInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FileVersion(writeTempPE(t, tc.data))
			if !errors.Is(err, tc.want) {
				t.Errorf("got err %v, want errors.Is %v", err, tc.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := FileVersion(filepath.Join(t.TempDir(), "nope.dll"))
		if err == nil {
			t.Fatal("want error for missing file")
		}
	})
}

func TestFileVersion_BoundedReads(t *testing.T) {
	t.Run("oversized file rejected via stat without slurping", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "huge.dll")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		// Sparse file just past the cap: stat rejects it before any read.
		if err := f.Truncate(maxPEFileSize + 1); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := FileVersion(p); !errors.Is(err, ErrTooLarge) {
			t.Errorf("got err %v, want errors.Is ErrTooLarge", err)
		}
	})

	t.Run("directory rejected as non-regular", func(t *testing.T) {
		if _, err := FileVersion(t.TempDir()); !errors.Is(err, ErrNotRegular) {
			t.Errorf("got err %v, want errors.Is ErrNotRegular", err)
		}
	})
}

func withBadSig(b []byte) []byte {
	c := append([]byte(nil), b...)
	c[0x40] = 'X'
	return c
}

func withBadOptMagic(b []byte) []byte {
	c := append([]byte(nil), b...)
	opt := 0x40 + 4 + 20
	binary.LittleEndian.PutUint16(c[opt:], 0x999)
	return c
}

func badStrucVersion() []byte {
	b := fixedFileInfo(1, 2, 3, 4)
	binary.LittleEndian.PutUint32(b[4:], 0x00042)
	return b
}

func TestGetFileVersionNormalization(t *testing.T) {
	cases := []struct {
		name    string
		fixed   string
		product string
		want    string
	}{
		{"product preferred", "1.2.3.4", "3.1.4", "3.1.4"},
		{"placeholder product 1.0.0.0 uses fixed", "2.4.12.0", "1.0.0.0", "2.4.12.0"},
		{"placeholder product 1.0 uses fixed", "2.4.12.0", "1.0", "2.4.12.0"},
		{"placeholder product 1.0.7 uses fixed", "2.4.12.0", "1.0.7", "2.4.12.0"},
		{"empty product uses fixed", "3.7.10.0", "", "3.7.10.0"},
		{"empty fixed uses product", "", "0.9.4", "0.9.4"},
		{"commas become dots", "", "3,1,4", "3.1.4"},
		{"FSR prefix stripped", "", "FSR 3.1.4", "3.1.4"},
		{"whitespace and v trimmed", "", "  v0.9.4 ", "0.9.4"},
		{"vendor suffix cut", "", "0.9.4-final (7534ad0)", "0.9.4"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalize(tc.fixed, tc.product); got != tc.want {
				t.Errorf("normalize(%q, %q) = %q, want %q", tc.fixed, tc.product, got, tc.want)
			}
		})
	}
}

func TestExtractTitle_ProductName(t *testing.T) {
	res := stringInfoString("ProductName", "My Cool Game")
	got := ExtractTitle(buildPE(true, res))
	if got != "My Cool Game" {
		t.Errorf("ExtractTitle = %q, want %q", got, "My Cool Game")
	}
}

func TestExtractTitle_FallsBackToFileDescription(t *testing.T) {
	res := concatStringStructs(stringInfoString("ProductName", "TODO: <Product Name>"),
		stringInfoString("FileDescription", "Fallback Title"))
	got := ExtractTitle(buildPE(false, res))
	if got != "Fallback Title" {
		t.Errorf("ExtractTitle = %q, want %q", got, "Fallback Title")
	}
}

func TestExtractTitle_RejectsGarbage(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"todo marker", "TODO"},
		{"angle placeholder", "<name>"},
		{"empty", ""},
		{"all digits", "12345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := stringInfoString("ProductName", tc.value)
			if got := ExtractTitle(buildPE(true, res)); got != "" {
				t.Errorf("ExtractTitle = %q, want %q", got, "")
			}
		})
	}

	t.Run("product name equal to company name", func(t *testing.T) {
		res := concatStringStructs(stringInfoString("ProductName", "Acme"),
			stringInfoString("CompanyName", "Acme"))
		if got := ExtractTitle(buildPE(true, res)); got != "" {
			t.Errorf("ExtractTitle = %q, want %q", got, "")
		}
	})
}

func TestExtractTitle_NoVersionInfo(t *testing.T) {
	if got := ExtractTitle([]byte("not a pe at all")); got != "" {
		t.Errorf("non-PE: ExtractTitle = %q, want %q", got, "")
	}
	if got := ExtractTitle(buildPE(false, []byte("deadbeefdeadbeef"))); got != "" {
		t.Errorf("no rsrc: ExtractTitle = %q, want %q", got, "")
	}
}

func TestScanStringFileInfoKey_CustomKey(t *testing.T) {
	res := stringInfoString("CustomKey", "CustomValue")
	if got := scanStringFileInfoKey(res, "CustomKey"); got != "CustomValue" {
		t.Errorf("scanStringFileInfoKey = %q, want %q", got, "CustomValue")
	}
	if got := scanStringFileInfoKey(res, "OtherKey"); got != "" {
		t.Errorf("missing key: scanStringFileInfoKey = %q, want %q", got, "")
	}
}

// --- TitleFromFile (ReaderAt, no whole-file read, no size cap) -----------

// writePEWithResourceAt writes a PE whose .rsrc raw pointer is patched to
// resOff and whose resource payload lands there, so tests control how far
// into the file the resource section sits.
func writePEWithResourceAt(t *testing.T, resData []byte, resOff int64) string {
	t.Helper()
	pe := buildPE(false, resData)
	// PE32 layout from buildPE: section table at 0x138, PointerToRawData at +20.
	const ptrRawDataField = 0x138 + 20
	binary.LittleEndian.PutUint32(pe[ptrRawDataField:], uint32(resOff))
	p := filepath.Join(t.TempDir(), "game.exe")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteAt(pe, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(resData, resOff); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTitleFromFile_ExtractsTitle(t *testing.T) {
	res := concatStringStructs(
		stringInfoString("CompanyName", "Acme"),
		stringInfoString("ProductName", "My Cool Game"),
	)
	if got := TitleFromFile(writeTempPE(t, buildPE(false, res))); got != "My Cool Game" {
		t.Errorf("TitleFromFile = %q, want %q", got, "My Cool Game")
	}
}

// The resource section can sit far past the header window: titles must be
// found without slurping the whole image.
func TestTitleFromFile_ResourceBeyondHeaderWindow(t *testing.T) {
	res := stringInfoString("ProductName", "Far Away Title")
	if got := TitleFromFile(writePEWithResourceAt(t, res, 1<<20)); got != "Far Away Title" {
		t.Errorf("TitleFromFile = %q, want %q", got, "Far Away Title")
	}
}

// AAA game exes exceed the old 128MiB whole-file cap (the user's
// Dead Space.exe is 423MB): a >128MiB exe must still yield its title.
func TestTitleFromFile_HugeFileNotSizeCapped(t *testing.T) {
	res := stringInfoString("ProductName", "Huge Game Title")
	p := writePEWithResourceAt(t, res, 150<<20)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() <= 128<<20 {
		t.Fatalf("fixture too small: %d bytes", fi.Size())
	}
	if got := TitleFromFile(p); got != "Huge Game Title" {
		t.Errorf("TitleFromFile = %q, want %q", got, "Huge Game Title")
	}
}

func TestTitleFromFile_NotPEOrMissing(t *testing.T) {
	if got := TitleFromFile(filepath.Join(t.TempDir(), "missing.exe")); got != "" {
		t.Errorf("missing file: TitleFromFile = %q, want %q", got, "")
	}
	p := writeTempPE(t, []byte("not a pe at all"))
	if got := TitleFromFile(p); got != "" {
		t.Errorf("non-PE: TitleFromFile = %q, want %q", got, "")
	}
}

// Unreal's generic bootstrap ships ProductName "BootstrapPackagedGame" in
// every packaged game — a placeholder, not a title (the user saw Tempest
// Rising and Empire of the Ants row under that name).
func TestTitleFromResource_RejectsBootstrapPackagedGame(t *testing.T) {
	for _, v := range []string{"BootstrapPackagedGame", "bootstrappackagedgame", "BOOTSTRAPPACKAGEDGAME"} {
		res := stringInfoString("ProductName", v)
		if got := ExtractTitle(buildPE(true, res)); got != "" {
			t.Errorf("ExtractTitle(%q) = %q, want %q", v, got, "")
		}
		if got := TitleFromFile(writeTempPE(t, buildPE(false, res))); got != "" {
			t.Errorf("TitleFromFile(%q) = %q, want %q", v, got, "")
		}
	}
}

// Unreal's other stock placeholder: packaged games that never set a
// ProductName ship "UE4Game" (the user saw Obduction row under it).
func TestTitleFromResource_RejectsUE4Game(t *testing.T) {
	for _, v := range []string{"UE4Game", "ue4game"} {
		res := stringInfoString("ProductName", v)
		if got := ExtractTitle(buildPE(true, res)); got != "" {
			t.Errorf("ExtractTitle(%q) = %q, want %q", v, got, "")
		}
	}
}

// Vendor-baked mojibake is not metadata: Helldivers 2's exe carries a
// ProductName whose ™ was destroyed at build time ("HELLDIVERSï¿½ 2" —
// the double-encoded replacement char). Treat mojibake strings as
// unusable so the clean FileDescription wins.
func TestExtractTitle_MojibakeFallsToFileDescription(t *testing.T) {
	res := concatStringStructs(
		stringInfoString("ProductName", "HELLDIVERS\u00ef\u00bf\u00bd 2"),
		stringInfoString("FileDescription", "HELLDIVERS 2"),
	)
	if got := ExtractTitle(buildPE(true, res)); got != "HELLDIVERS 2" {
		t.Errorf("ExtractTitle = %q, want %q (mojibake ProductName rejected)", got, "HELLDIVERS 2")
	}
	res = concatStringStructs(
		stringInfoString("ProductName", "Broken \ufffd Game"),
		stringInfoString("FileDescription", "Clean Title"),
	)
	if got := ExtractTitle(buildPE(true, res)); got != "Clean Title" {
		t.Errorf("ExtractTitle = %q, want %q (replacement char rejected)", got, "Clean Title")
	}
}

// Vendor-junk metadata is not a game title: repacked exes and tool
// launchers carry strings like "Electronic Arts System Information" that
// beat the folder name for no good reason. Rejected → the chain falls
// through to FileDescription/stem/folder.
func TestExtractTitle_RejectsVendorJunk(t *testing.T) {
	for _, v := range []string{"Electronic Arts System Information", "Shockwave Flash", "Elevate Application", "Easy MFC Application", "Macromedia Flash Player 8.0  r22", "Elevate"} {
		res := concatStringStructs(
			stringInfoString("ProductName", v),
			stringInfoString("FileDescription", "Real Game"),
		)
		if got := ExtractTitle(buildPE(true, res)); got != "Real Game" {
			t.Errorf("ExtractTitle(%q) = %q, want %q (junk ProductName rejected)", v, got, "Real Game")
		}
	}
}
