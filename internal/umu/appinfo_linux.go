//go:build linux

package umu

import (
	"bytes"
	"encoding/binary"
	"os"
)

// appinfo.vdf layouts: the legacy 0x07564427 (8-byte header, single-SHA
// record headers, NUL-terminated key strings) and the string-pool
// 0x07564428/0x07564429 (16-byte header with the pool offset, dual-SHA
// record headers, uint32 pool indices as keys).
const (
	appinfoMagicLegacy = 0x07564427
	appinfoMagicPool28 = 0x07564428
	appinfoMagicPool29 = 0x07564429
)

// appinfoNames parses a Steam appcache/appinfo.vdf into appid → display
// name (common/name of each app record). It covers every app Steam has
// cached on the machine — including uninstalled games whose compatdata
// outlived their appmanifest (issue 074). nil on any unreadable,
// unknown-layout, or corrupt content: the caller falls back to the raw
// appid.
func appinfoNames(path string) map[uint32]string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 8 {
		return nil
	}
	p := &appinfoParser{data: data, end: len(data)}
	switch binary.LittleEndian.Uint32(data) {
	case appinfoMagicLegacy:
		p.pos, p.recHeader = 8, 40
	case appinfoMagicPool28, appinfoMagicPool29:
		if len(data) < 16 {
			return nil
		}
		poolOff := int(binary.LittleEndian.Uint64(data[8:]))
		if poolOff < 16 || poolOff+4 > len(data) {
			return nil
		}
		p.pos, p.recHeader, p.end = 16, 60, poolOff
		if p.pool = readStringPool(data, poolOff); p.pool == nil {
			return nil
		}
	default:
		return nil
	}
	out := map[uint32]string{}
	for p.pos+8 <= p.end {
		recStart := p.pos
		appid := binary.LittleEndian.Uint32(data[recStart:])
		size := int(binary.LittleEndian.Uint32(data[recStart+4:]))
		if size < p.recHeader || recStart+8+size > p.end {
			return nil
		}
		p.pos = recStart + 8 + p.recHeader
		kv, ok := p.parseMap(recStart+8+size, 0)
		if !ok {
			return nil
		}
		if name := appinfoRecordName(kv); name != "" {
			out[appid] = name
		}
		p.pos = recStart + 8 + size
	}
	return out
}

// readStringPool reads the pool at off: a uint32 count followed by that
// many NUL-terminated strings. nil when the pool overruns the file.
func readStringPool(data []byte, off int) []string {
	n := int(binary.LittleEndian.Uint32(data[off:]))
	pool := make([]string, 0, n)
	pos := off + 4
	for i := 0; i < n; i++ {
		e := bytes.IndexByte(data[pos:], 0)
		if e < 0 {
			return nil
		}
		pool = append(pool, string(data[pos:pos+e]))
		pos += e + 1
	}
	return pool
}

// appinfoParser walks one appinfo.vdf. pos is the cursor, end the end of
// the records region (the string pool follows in pool layouts), recHeader
// the per-record fixed header size past appid+size, pool the shared key
// strings (nil in the legacy layout, where keys are inline cstrings).
type appinfoParser struct {
	data      []byte
	pos, end  int
	recHeader int
	pool      []string
}

// parseMap parses binary-VDF entries until the 0x08 terminator or the
// limit offset, whichever comes first. Only submaps and strings are
// kept; ints/floats are skipped over. ok is false on any malformed or
// unknown entry — appinfo.vdf uses only the types handled here.
func (p *appinfoParser) parseMap(limit, depth int) (map[string]any, bool) {
	if depth > 16 {
		return nil, false
	}
	out := map[string]any{}
	for p.pos < limit {
		typ := p.data[p.pos]
		p.pos++
		if typ == 0x08 {
			return out, true
		}
		var key string
		if p.pool != nil {
			if p.pos+4 > limit {
				return nil, false
			}
			idx := binary.LittleEndian.Uint32(p.data[p.pos:])
			p.pos += 4
			if int(idx) >= len(p.pool) {
				return nil, false
			}
			key = p.pool[idx]
		} else {
			e := bytes.IndexByte(p.data[p.pos:limit], 0)
			if e < 0 {
				return nil, false
			}
			key = string(p.data[p.pos : p.pos+e])
			p.pos += e + 1
		}
		switch typ {
		case 0x00: // submap
			sub, ok := p.parseMap(limit, depth+1)
			if !ok {
				return nil, false
			}
			out[key] = sub
		case 0x01: // cstring
			e := bytes.IndexByte(p.data[p.pos:limit], 0)
			if e < 0 {
				return nil, false
			}
			out[key] = string(p.data[p.pos : p.pos+e])
			p.pos += e + 1
		case 0x02, 0x03, 0x06: // int32, float32, color
			p.pos += 4
		case 0x07, 0x0A: // uint64, int64
			p.pos += 8
		default:
			return nil, false
		}
		if p.pos > limit {
			return nil, false
		}
	}
	return out, true
}

// appinfoRecordName digs common/name out of a parsed app record; newer
// files wrap the record in an "appinfo" key, legacy ones do not.
func appinfoRecordName(kv map[string]any) string {
	if sub, ok := kv["appinfo"].(map[string]any); ok {
		kv = sub
	}
	if common, ok := kv["common"].(map[string]any); ok {
		if name, ok := common["name"].(string); ok {
			return name
		}
	}
	return ""
}
