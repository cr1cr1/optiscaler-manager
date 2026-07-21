package pever

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

const (
	vsFixedFileInfoSig      = 0xFEEF04BD
	vsFixedFileInfoStrucVer = 0x00010000
	vsFixedFileInfoLen      = 52
	optMagicPE32            = 0x10B
	optMagicPE32Plus        = 0x20B
	dirEntryResource        = 2
	maxSections             = 96
	maxVersionScan          = 4 << 20
	stringValueMaxChars     = 256
)

// parsePEVersion extracts the fixed FILEVERSION dotted quad and the
// ProductVersion string from a PE image. Either may be empty. Structural
// failures yield ErrNotPE; a valid PE with no version data yields
// ErrNoVersionInfo.
func parsePEVersion(data []byte) (fixed, product string, err error) {
	res, err := resourceBytes(data)
	if err != nil {
		return "", "", err
	}
	if len(res) > maxVersionScan {
		res = res[:maxVersionScan]
	}
	fixed = scanFixedFileInfo(res)
	product = scanProductVersion(res)
	if fixed == "" && product == "" {
		return "", "", ErrNoVersionInfo
	}
	return fixed, product, nil
}

// section is one IMAGE_SECTION_HEADER field triple needed for RVA mapping.
type section struct {
	virtualAddress uint32
	sizeOfRawData  uint32
	ptrRawData     uint32
}

// resourceBytes walks the PE headers and returns the raw bytes of the
// resource (DataDirectory[2]) region, clipped to the file. Every offset is
// bounds-checked; any structural anomaly returns ErrNotPE.
func resourceBytes(data []byte) ([]byte, error) {
	off, size, err := resourceRange(data)
	if err != nil {
		return nil, err
	}
	end := off + size
	if end > len(data) {
		end = len(data)
	}
	if end <= off {
		return nil, ErrNoVersionInfo
	}
	return data[off:end], nil
}

// resourceRange is resourceBytes for callers that read windows of a larger
// file: it returns the resource region's file offset and declared size
// without slicing data. data must hold the complete PE header block (DOS
// header, PE signature, COFF + optional headers, section table).
func resourceRange(data []byte) (off, size int, err error) {
	u16 := func(off int) (uint16, bool) {
		if off < 0 || off+2 > len(data) {
			return 0, false
		}
		return binary.LittleEndian.Uint16(data[off:]), true
	}
	u32 := func(off int) (uint32, bool) {
		if off < 0 || off+4 > len(data) {
			return 0, false
		}
		return binary.LittleEndian.Uint32(data[off:]), true
	}

	mz, ok := u16(0)
	if !ok || mz != 0x5A4D { // "MZ"
		return 0, 0, ErrNotPE
	}
	lfanew, ok := u32(0x3C)
	if !ok {
		return 0, 0, ErrNotPE
	}
	pe := int(lfanew)
	sig, ok := u32(pe)
	if !ok || sig != 0x00004550 { // "PE\0\0"
		return 0, 0, ErrNotPE
	}
	coff := pe + 4
	numSections, ok := u16(coff + 2)
	if !ok || numSections == 0 || numSections > maxSections {
		return 0, 0, ErrNotPE
	}
	optSize, ok := u16(coff + 16)
	if !ok {
		return 0, 0, ErrNotPE
	}
	opt := coff + 20
	if opt+int(optSize) > len(data) {
		return 0, 0, ErrNotPE
	}
	magic, ok := u16(opt)
	if !ok {
		return 0, 0, ErrNotPE
	}
	var ddBase int
	switch magic {
	case optMagicPE32:
		ddBase = opt + 96
	case optMagicPE32Plus:
		ddBase = opt + 112
	default:
		return 0, 0, ErrNotPE
	}
	resDD := ddBase + dirEntryResource*8
	if resDD+8 > opt+int(optSize) {
		return 0, 0, ErrNoVersionInfo // optional header has no resource entry
	}
	resRVA, _ := u32(resDD)
	resSize, _ := u32(resDD + 4)
	if resRVA == 0 || resSize == 0 {
		return 0, 0, ErrNoVersionInfo
	}

	secTab := opt + int(optSize)
	sections := make([]section, 0, numSections)
	for i := 0; i < int(numSections); i++ {
		sh := secTab + i*40
		if sh+40 > len(data) {
			return 0, 0, ErrNotPE
		}
		va, _ := u32(sh + 12)
		rawSize, _ := u32(sh + 16)
		rawPtr, _ := u32(sh + 20)
		sections = append(sections, section{va, rawSize, rawPtr})
	}

	roff, ok := rvaToOffset(sections, resRVA)
	if !ok {
		return 0, 0, ErrNoVersionInfo
	}
	return int(roff), int(resSize), nil
}

func rvaToOffset(sections []section, rva uint32) (uint32, bool) {
	for _, s := range sections {
		if rva >= s.virtualAddress && rva-s.virtualAddress < s.sizeOfRawData {
			return s.ptrRawData + (rva - s.virtualAddress), true
		}
	}
	return 0, false
}

// scanFixedFileInfo searches for a VS_FIXEDFILEINFO signature and returns
// the dotted quad from the first block with a valid dwStrucVersion.
func scanFixedFileInfo(res []byte) string {
	sig := []byte{0xBD, 0x04, 0xEF, 0xFE}
	for i := 0; ; {
		j := bytes.Index(res[i:], sig)
		if j < 0 {
			return ""
		}
		p := i + j
		if p+vsFixedFileInfoLen <= len(res) &&
			binary.LittleEndian.Uint32(res[p+4:]) == vsFixedFileInfoStrucVer {
			ms := binary.LittleEndian.Uint32(res[p+8:])
			ls := binary.LittleEndian.Uint32(res[p+12:])
			return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xFFFF, ls>>16, ls&0xFFFF)
		}
		i = p + 4
		if i >= len(res) {
			return ""
		}
	}
}

// scanProductVersion finds the StringFileInfo entry "ProductVersion" and
// decodes its UTF-16LE value.
func scanProductVersion(res []byte) string {
	return scanStringFileInfoKey(res, "ProductVersion")
}

// utf16Key encodes an ASCII StringFileInfo key as UTF-16LE bytes with a NUL
// terminator, matching the szKey layout of a String structure.
func utf16Key(key string) []byte {
	b := make([]byte, 0, len(key)*2+2)
	for i := 0; i < len(key); i++ {
		b = append(b, key[i], 0)
	}
	return append(b, 0, 0)
}

// scanStringFileInfoKey finds the StringFileInfo entry for key and decodes
// its UTF-16LE value. The String structure layout is
// wLength(2) wValueLength(2) wType(2) szKey... padding Value, with members
// aligned to 4-byte boundaries relative to the version resource base;
// resource sections are always 4-aligned in the file, so file offsets are
// used for alignment here.
func scanStringFileInfoKey(res []byte, key string) string {
	kb := utf16Key(key)
	p := bytes.Index(res, kb)
	if p < 0 || p < 6 {
		return ""
	}
	valueWords := int(binary.LittleEndian.Uint16(res[p-4:]))
	wType := binary.LittleEndian.Uint16(res[p-2:])
	if wType != 1 || valueWords <= 0 || valueWords > stringValueMaxChars {
		return ""
	}
	valOff := (p + len(kb) + 3) &^ 3
	valLen := valueWords * 2
	if valOff+valLen > len(res) {
		return ""
	}
	raw := res[valOff : valOff+valLen]
	u16s := make([]uint16, 0, valueWords)
	for i := 0; i+2 <= len(raw); i += 2 {
		w := binary.LittleEndian.Uint16(raw[i:])
		if w == 0 {
			break
		}
		u16s = append(u16s, w)
	}
	return string(utf16.Decode(u16s))
}

// ExtractTitle returns a human-readable game title from a PE image's
// StringFileInfo: ProductName wins, FileDescription is the fallback.
// Placeholder values (empty, a "todo" prefix, <...> wrappers, all-digit
// strings, the Unreal bootstrap name, or the CompanyName repeated) are
// rejected; "" means no usable title.
func ExtractTitle(data []byte) (title string) {
	res, err := resourceBytes(data)
	if err != nil {
		return ""
	}
	return titleFromResource(res)
}

// headerWindow is how much of a PE file TitleFromFile reads for the header
// block (DOS stub, PE/COFF/optional headers, section table); real images
// keep these within the first few kilobytes.
const headerWindow = 64 << 10

// TitleFromFile is ExtractTitle for a path, reading only the header window
// and the resource region (each bounded) instead of the whole file: game
// exes of any size yield their title. "" means no usable title.
func TitleFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	window := make([]byte, headerWindow)
	n, _ := f.ReadAt(window, 0) // short read for small files is fine
	off, size, err := resourceRange(window[:n])
	if err != nil {
		return ""
	}
	if size < 0 {
		return "" // corrupt header arithmetic; never allocate from it
	}
	if size > maxVersionScan {
		size = maxVersionScan
	}
	buf := make([]byte, size)
	m, _ := f.ReadAt(buf, int64(off))
	return titleFromResource(buf[:m])
}

// titleFromResource scans resource-section bytes for a usable title:
// ProductName, then FileDescription, with CompanyName for the
// self-referential placeholder check.
func titleFromResource(res []byte) string {
	if len(res) > maxVersionScan {
		res = res[:maxVersionScan]
	}
	company := scanStringFileInfoKey(res, "CompanyName")
	for _, key := range []string{"ProductName", "FileDescription"} {
		if t := usableTitle(scanStringFileInfoKey(res, key), company); t != "" {
			return t
		}
	}
	return ""
}

// usableTitle filters vendor placeholder strings out of title candidates.
func usableTitle(v, company string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(v), "todo") {
		return ""
	}
	if len(v) >= 2 && v[0] == '<' && v[len(v)-1] == '>' {
		return ""
	}
	if strings.ContainsRune(v, '\ufffd') || strings.Contains(v, "\u00ef\u00bf\u00bd") {
		return "" // vendor-baked encoding damage is not metadata
	}
	if junkTitle(v) {
		return ""
	}
	if allDigit(v) {
		return ""
	}
	if strings.EqualFold(v, "bootstrappackagedgame") || strings.EqualFold(v, "ue4game") {
		return ""
	}
	if company != "" && v == company {
		return ""
	}
	return v
}

// junkTitle reports vendor metadata that names a tool or platform, never
// the game (repacked-exe strings, runtime hosts, launcher stubs,
// emulators).
func junkTitle(v string) bool {
	switch strings.ToLower(v) {
	case "electronic arts system information", "shockwave flash",
		"elevate application", "elevate", "easy mfc application",
		"cemu", "yuzu", "ryujinx", "dolphin", "pcsx2", "rpcs3",
		"xenia", "citra", "retroarch", "wii u emulator":
		return true
	}
	return strings.HasPrefix(strings.ToLower(v), "macromedia flash player")
}

func allDigit(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return len(v) > 0
}

// CompanyFromFile is TitleFromFile for the CompanyName string: the same
// bounded windowed reads, returning "" when no usable company is present.
func CompanyFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	window := make([]byte, headerWindow)
	n, _ := f.ReadAt(window, 0)
	off, size, err := resourceRange(window[:n])
	if err != nil || size < 0 {
		return ""
	}
	if size > maxVersionScan {
		size = maxVersionScan
	}
	buf := make([]byte, size)
	m, _ := f.ReadAt(buf, int64(off))
	res := buf[:m]
	if len(res) > maxVersionScan {
		res = res[:maxVersionScan]
	}
	return scanStringFileInfoKey(res, "CompanyName")
}
