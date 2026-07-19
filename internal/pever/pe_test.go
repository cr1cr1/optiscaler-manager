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

// productVersionString builds a minimal String structure for the
// StringFileInfo entry "ProductVersion" = value, 4-byte aligned at off 0.
func productVersionString(value string) []byte {
	key := utf16le("ProductVersion") // 28 bytes + 2 for NUL = 30
	val := utf16le(value)
	valWords := len(val)/2 + 1
	structLen := 6 + 30 + len(val) + 2
	b := make([]byte, structLen)
	binary.LittleEndian.PutUint16(b[0:], uint16(structLen))
	binary.LittleEndian.PutUint16(b[2:], uint16(valWords))
	binary.LittleEndian.PutUint16(b[4:], 1) // wType = text
	copy(b[6:], key)
	// key+NUL occupies b[6:36]; value starts at the 4-aligned offset 36.
	copy(b[36:], val)
	return b
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
