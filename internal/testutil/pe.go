// Package testutil holds fixtures shared by tests across packages. It is
// imported by *_test.go files only.
package testutil

import "encoding/binary"

// FixedVersionPE builds a minimal PE32 image whose .rsrc section carries a
// VS_FIXEDFILEINFO block for the dotted quad maj.min.patch.build, so
// pever.FileVersion parses it back as "maj.min.patch.build".
func FixedVersionPE(maj, min, patch, build uint16) []byte {
	const (
		eLfanew    = 0x40
		sectVA     = 0x1000
		sectRawOff = 0x200
		optSize    = 0xE0 // PE32: 96 + 16*8
	)
	res := fixedFileInfo(maj, min, patch, build)
	b := make([]byte, sectRawOff+len(res))

	// DOS header.
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3C:], eLfanew)

	// PE signature + COFF header.
	copy(b[eLfanew:], "PE\x00\x00")
	coff := eLfanew + 4
	binary.LittleEndian.PutUint16(b[coff:], 0x14C)
	binary.LittleEndian.PutUint16(b[coff+2:], 1)
	binary.LittleEndian.PutUint16(b[coff+16:], optSize)

	// Optional header (PE32 magic; resource data directory at index 2).
	opt := coff + 20
	binary.LittleEndian.PutUint16(b[opt:], 0x10B)
	dd := opt + 96
	binary.LittleEndian.PutUint32(b[dd+2*8:], sectVA)
	binary.LittleEndian.PutUint32(b[dd+2*8+4:], uint32(len(res)))

	// Section table (.rsrc).
	sec := opt + optSize
	copy(b[sec:], ".rsrc\x00\x00\x00")
	binary.LittleEndian.PutUint32(b[sec+8:], uint32(len(res)))
	binary.LittleEndian.PutUint32(b[sec+12:], sectVA)
	binary.LittleEndian.PutUint32(b[sec+16:], uint32(len(res)))
	binary.LittleEndian.PutUint32(b[sec+20:], sectRawOff)

	copy(b[sectRawOff:], res)
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
