//go:build linux

package umu

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildAppinfoPoolVDF assembles a synthetic appinfo.vdf in the
// 0x07564429 string-pool layout: 16-byte header (magic, universe, pool
// offset), then per-app records whose binary-VDF keys are uint32 pool
// indices, then the pool (count + NUL-terminated strings).
func buildAppinfoPoolVDF(t *testing.T, names map[uint32]string) []byte {
	t.Helper()
	pool := []string{"appinfo", "common", "name"}
	var records bytes.Buffer
	for appid, name := range names {
		var body bytes.Buffer
		body.WriteByte(0x00) // submap
		_ = binary.Write(&body, binary.LittleEndian, uint32(0))
		body.WriteByte(0x00)
		_ = binary.Write(&body, binary.LittleEndian, uint32(1))
		body.WriteByte(0x01) // string
		_ = binary.Write(&body, binary.LittleEndian, uint32(2))
		body.WriteString(name + "\x00")
		body.WriteByte(0x08)
		body.WriteByte(0x08)
		body.WriteByte(0x08)
		_ = binary.Write(&records, binary.LittleEndian, appid)
		_ = binary.Write(&records, binary.LittleEndian, uint32(60+body.Len()))
		records.Write(make([]byte, 4))  // infostate
		records.Write(make([]byte, 4))  // lastupdated
		records.Write(make([]byte, 8))  // picsaccesstoken
		records.Write(make([]byte, 20)) // sha_text
		records.Write(make([]byte, 4))  // changenumber
		records.Write(make([]byte, 20)) // sha_binary
		records.Write(body.Bytes())
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint32(0x07564429))
	_ = binary.Write(&out, binary.LittleEndian, uint32(1)) // universe
	_ = binary.Write(&out, binary.LittleEndian, int64(16+records.Len()))
	out.Write(records.Bytes())
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(pool)))
	for _, s := range pool {
		out.WriteString(s + "\x00")
	}
	return out.Bytes()
}

// buildAppinfoLegacyVDF assembles a synthetic appinfo.vdf in the legacy
// 0x07564427 layout: 8-byte header, single-SHA record headers, and
// NUL-terminated key strings in the binary VDF body.
func buildAppinfoLegacyVDF(t *testing.T, names map[uint32]string) []byte {
	t.Helper()
	var records bytes.Buffer
	for appid, name := range names {
		var body bytes.Buffer
		body.WriteByte(0x00)
		body.WriteString("appinfo\x00")
		body.WriteByte(0x00)
		body.WriteString("common\x00")
		body.WriteByte(0x01)
		body.WriteString("name\x00")
		body.WriteString(name + "\x00")
		body.WriteByte(0x08)
		body.WriteByte(0x08)
		body.WriteByte(0x08)
		_ = binary.Write(&records, binary.LittleEndian, appid)
		_ = binary.Write(&records, binary.LittleEndian, uint32(40+body.Len()))
		records.Write(make([]byte, 4))  // infostate
		records.Write(make([]byte, 4))  // lastupdated
		records.Write(make([]byte, 8))  // picsaccesstoken
		records.Write(make([]byte, 20)) // sha_text
		records.Write(make([]byte, 4))  // changenumber
		records.Write(body.Bytes())
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint32(0x07564427))
	_ = binary.Write(&out, binary.LittleEndian, uint32(1)) // universe
	out.Write(records.Bytes())
	return out.Bytes()
}

func TestAppinfoNames_StringPoolFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appinfo.vdf")
	data := buildAppinfoPoolVDF(t, map[uint32]string{
		312520: "Rain World",
		578080: "PUBG: BATTLEGROUNDS",
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := appinfoNames(path)
	if got[312520] != "Rain World" || got[578080] != "PUBG: BATTLEGROUNDS" {
		t.Errorf("appinfoNames = %v, want appids mapped to display names", got)
	}
}

func TestAppinfoNames_LegacyFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appinfo.vdf")
	data := buildAppinfoLegacyVDF(t, map[uint32]string{312520: "Rain World"})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := appinfoNames(path)
	if got[312520] != "Rain World" {
		t.Errorf("appinfoNames = %v, want 312520 -> Rain World", got)
	}
}

func TestAppinfoNames_GarbageReturnsNil(t *testing.T) {
	if got := appinfoNames(filepath.Join(t.TempDir(), "missing.vdf")); got != nil {
		t.Errorf("missing file: got %v, want nil", got)
	}
	for name, data := range map[string][]byte{
		"bad magic":  []byte("not an appinfo file at all........"),
		"truncated":  buildAppinfoPoolVDF(t, map[uint32]string{312520: "Rain World"})[:40],
		"empty pool": {0x29, 0x44, 0x56, 0x07, 1, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0},
	} {
		path := filepath.Join(t.TempDir(), "appinfo.vdf")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := appinfoNames(path); got != nil {
			t.Errorf("%s: got %v, want nil", name, got)
		}
	}
}

func TestFindPrefixes_SteamAppinfoFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	steamapps := filepath.Join(home, ".steam", "steam", "steamapps")
	// No appmanifest, but appinfo.vdf knows the title (uninstalled game
	// with leftover compatdata — the reported bug).
	orphan := mkPrefix(t, filepath.Join(steamapps, "compatdata", "312520", "pfx"))
	// Appmanifest present: it wins over appinfo.vdf.
	manifested := mkPrefix(t, filepath.Join(steamapps, "compatdata", "232790", "pfx"))
	writeFile(t, filepath.Join(steamapps, "appmanifest_232790.acf"),
		"\"AppState\"\n{\n\t\"appid\"\t\t\"232790\"\n\t\"name\"\t\t\"Broken Age\"\n}\n")
	// Neither manifest nor appinfo entry: the raw appid stays.
	raw := mkPrefix(t, filepath.Join(steamapps, "compatdata", "99999", "pfx"))

	writeFile(t, filepath.Join(home, ".steam", "steam", "appcache", "appinfo.vdf"),
		string(buildAppinfoPoolVDF(t, map[uint32]string{
			312520: "Rain World",
			232790: "Wrong Name",
		})))

	got := FindPrefixes(context.Background())
	byPath := map[string]Prefix{}
	for _, p := range got {
		byPath[p.Path] = p
	}
	checks := []struct{ path, name string }{
		{orphan, "Rain World"},
		{manifested, "Broken Age"},
		{raw, "99999"},
	}
	for _, c := range checks {
		p, ok := byPath[c.path]
		if !ok {
			t.Errorf("FindPrefixes missing %q (got %v)", c.path, got)
			continue
		}
		if p.Name != c.name {
			t.Errorf("prefix %q Name = %q, want %q", c.path, p.Name, c.name)
		}
	}
}
