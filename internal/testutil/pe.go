// Package testutil holds fixtures shared by tests across packages. It is
// imported by *_test.go files only.
package testutil

import (
	"encoding/binary"
	"sort"
)

// FixedVersionPE builds a minimal PE32 image whose .rsrc section carries a
// VS_FIXEDFILEINFO block for the dotted quad maj.min.patch.build, so
// pever.FileVersion parses it back as "maj.min.patch.build".
func FixedVersionPE(maj, min, patch, build uint16) []byte {
	return buildPEImage(false, fixedFileInfo(maj, min, patch, build))
}

// StringInfoPE builds a minimal PE32 (pe32plus=false) or PE32+ image whose
// .rsrc section carries a VS_FIXEDFILEINFO block for fileVersion — omitted
// when fileVersion is all zero — plus one 4-byte-aligned StringFileInfo
// String structure per entry in strings. Keys are sorted so the layout is
// deterministic. Use it to plant synthetic upscaler/injection DLLs in
// discovery fixtures.
func StringInfoPE(pe32plus bool, strings map[string]string, fileVersion [4]uint16) []byte {
	var res []byte
	if fileVersion != ([4]uint16{}) {
		res = append(res, fixedFileInfo(fileVersion[0], fileVersion[1], fileVersion[2], fileVersion[3])...)
	}
	keys := make([]string, 0, len(strings))
	for k := range strings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		res = append(res, stringInfoString(k, strings[k])...)
		for len(res)%4 != 0 {
			res = append(res, 0)
		}
	}
	return buildPEImage(pe32plus, res)
}

// buildPEImage wraps resData in a minimal PE32/PE32+ image with a single
// .rsrc section; the resource data-directory entry points at the whole
// section payload.
func buildPEImage(pe32plus bool, resData []byte) []byte {
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
	b := make([]byte, sectRawOff+len(resData))

	// DOS header.
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3C:], eLfanew)

	// PE signature + COFF header.
	copy(b[eLfanew:], "PE\x00\x00")
	coff := eLfanew + 4
	binary.LittleEndian.PutUint16(b[coff:], 0x14C)
	binary.LittleEndian.PutUint16(b[coff+2:], 1)
	binary.LittleEndian.PutUint16(b[coff+16:], uint16(optSize))

	// Optional header (resource data directory at index 2).
	opt := coff + 20
	binary.LittleEndian.PutUint16(b[opt:], magic)
	dd := opt + ddOff
	binary.LittleEndian.PutUint32(b[dd+2*8:], sectVA)
	binary.LittleEndian.PutUint32(b[dd+2*8+4:], uint32(len(resData)))

	// Section table (.rsrc).
	sec := opt + optSize
	copy(b[sec:], ".rsrc\x00\x00\x00")
	binary.LittleEndian.PutUint32(b[sec+8:], uint32(len(resData)))
	binary.LittleEndian.PutUint32(b[sec+12:], sectVA)
	binary.LittleEndian.PutUint32(b[sec+16:], uint32(len(resData)))
	binary.LittleEndian.PutUint32(b[sec+20:], sectRawOff)

	copy(b[sectRawOff:], resData)
	return b
}

func fixedFileInfo(maj, min, patch, build uint16) []byte {
	b := make([]byte, 52)
	binary.LittleEndian.PutUint32(b[0:], 0xFEEF04BD)
	binary.LittleEndian.PutUint32(b[4:], 0x00010000) // dwStrucVersion
	binary.LittleEndian.PutUint32(b[8:], uint32(maj)<<16|uint32(min))
	binary.LittleEndian.PutUint32(b[12:], uint32(patch)<<16|uint32(build))
	return b
}

// utf16le encodes s as UTF-16LE bytes (no terminator).
func utf16le(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r > 0xFFFF {
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
	kb := utf16le(key)
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
