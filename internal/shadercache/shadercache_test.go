package shadercache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSized(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byKind(refs []Ref, k Kind) *Ref {
	for i := range refs {
		if refs[i].Kind == k {
			return &refs[i]
		}
	}
	return nil
}

// Known cache files in the game dir are claimed per-game with size and
// freshness; nothing is ever deleted by discovery.
func TestDiscoverClaimsKnownFiles(t *testing.T) {
	game := t.TempDir()
	writeSized(t, filepath.Join(game, "game.dxvk-cache"), 240)
	writeSized(t, filepath.Join(game, "vkd3d-proton.cache"), 100)
	before := time.Now()

	refs := Discover(Input{GameDir: game, HomeDir: t.TempDir()})

	dxvk := byKind(refs, KindDXVK)
	if dxvk == nil {
		t.Fatalf("no DXVK ref in %+v", refs)
	}
	if dxvk.Scope != ScopePerGame || dxvk.SizeBytes != 240 {
		t.Errorf("dxvk = %+v, want per-game 240 bytes", dxvk)
	}
	if dxvk.ModTime.Before(before.Add(-time.Minute)) || dxvk.ModTime.After(time.Now().Add(time.Minute)) {
		t.Errorf("dxvk modtime = %v, want ~now", dxvk.ModTime)
	}
	vkd3d := byKind(refs, KindVKD3D)
	if vkd3d == nil || vkd3d.SizeBytes != 100 || vkd3d.Scope != ScopePerGame {
		t.Errorf("vkd3d = %+v, want per-game 100 bytes", vkd3d)
	}
}

// The manager's own backup stores and VCS dirs are never shader caches.
func TestDiscoverSkipsOwnStores(t *testing.T) {
	game := t.TempDir()
	writeSized(t, filepath.Join(game, "optiscaler-backups", "files", "x.dxvk-cache"), 10)
	writeSized(t, filepath.Join(game, "dlss-backups", "x.dxvk-cache"), 10)
	writeSized(t, filepath.Join(game, ".git", "x.dxvk-cache"), 10)

	if refs := Discover(Input{GameDir: game, HomeDir: t.TempDir()}); len(refs) != 0 {
		t.Errorf("refs = %+v, want none", refs)
	}
}

// Prefixes are huge: only shallow cache files count; deep prefix noise
// is ignored.
func TestDiscoverPrefixShallowOnly(t *testing.T) {
	game := t.TempDir()
	prefix := t.TempDir()
	writeSized(t, filepath.Join(prefix, "game.dxvk-cache"), 50)
	writeSized(t, filepath.Join(prefix, "drive_c", "deep", "game.dxvk-cache"), 60)

	refs := Discover(Input{GameDir: game, PrefixDir: prefix, HomeDir: t.TempDir()})

	dxvk := byKind(refs, KindDXVK)
	if dxvk == nil || dxvk.SizeBytes != 50 {
		t.Errorf("dxvk = %+v, want the shallow 50-byte file only", dxvk)
	}
}

// A child cache dir of unknown format is reported info-only (sized, never
// cleared); a dir holding claimed files reports the files, not the dir.
func TestDiscoverUnclaimedDirs(t *testing.T) {
	game := t.TempDir()
	writeSized(t, filepath.Join(game, "cache", "0000_0004", "blob.bin"), 1300)
	writeSized(t, filepath.Join(game, "saves", "slot.sav"), 5)

	refs := Discover(Input{GameDir: game, HomeDir: t.TempDir()})

	u := byKind(refs, KindUnclaimed)
	if u == nil {
		t.Fatalf("no unclaimed ref in %+v", refs)
	}
	if u.Scope != ScopePerGame || u.SizeBytes != 1300 {
		t.Errorf("unclaimed = %+v, want per-game 1300 bytes", u)
	}
	if got := filepath.Base(u.Path); got != "cache" {
		t.Errorf("unclaimed path = %v, want the cache dir", u.Path)
	}

	game2 := t.TempDir()
	writeSized(t, filepath.Join(game2, "cache", "game.dxvk-cache"), 70)
	refs2 := Discover(Input{GameDir: game2, HomeDir: t.TempDir()})
	if u2 := byKind(refs2, KindUnclaimed); u2 != nil {
		t.Errorf("unclaimed = %+v, want none when claimed files exist", u2)
	}
	if dxvk := byKind(refs2, KindDXVK); dxvk == nil || dxvk.SizeBytes != 70 {
		t.Errorf("dxvk = %+v, want the 70-byte file", dxvk)
	}

	// Claimed files nested deeper inside an unknown-format dir are still
	// found (the walk is full, not pruned).
	game3 := t.TempDir()
	writeSized(t, filepath.Join(game3, "cache", "sub", "deep.dxvk-cache"), 33)
	refs3 := Discover(Input{GameDir: game3, HomeDir: t.TempDir()})
	if dxvk := byKind(refs3, KindDXVK); dxvk == nil || dxvk.SizeBytes != 33 {
		t.Errorf("dxvk = %+v, want the nested 33-byte file", dxvk)
	}
	if u3 := byKind(refs3, KindUnclaimed); u3 != nil {
		t.Errorf("unclaimed = %+v, want none when claimed files exist", u3)
	}
}

// Driver caches are global blobs: measured from home, shared scope, and
// omitted when absent.
func TestDiscoverGlobalCaches(t *testing.T) {
	home := t.TempDir()
	writeSized(t, filepath.Join(home, ".cache", "mesa_shader_cache", "a", "f"), 190)
	writeSized(t, filepath.Join(home, ".cache", "nvidia", "GLCache", "g"), 250)

	refs := Discover(Input{GameDir: t.TempDir(), HomeDir: home})

	mesa := byKind(refs, KindMesa)
	if mesa == nil || mesa.Scope != ScopeGlobal || mesa.SizeBytes != 190 {
		t.Errorf("mesa = %+v, want global 190 bytes", mesa)
	}
	nv := byKind(refs, KindNvidia)
	if nv == nil || nv.Scope != ScopeGlobal || nv.SizeBytes != 250 {
		t.Errorf("nvidia = %+v, want global 250 bytes", nv)
	}
	if refs := Discover(Input{GameDir: t.TempDir(), HomeDir: t.TempDir()}); len(refs) != 0 {
		t.Errorf("refs = %+v, want none without caches", refs)
	}
}

// Steam precache belongs to Steam: measured read-only per appid, omitted
// when never fetched (the pane renders "not fetched" from the appid).
func TestDiscoverSteamFoz(t *testing.T) {
	home := t.TempDir()
	foz := filepath.Join(home, ".local", "share", "Steam", "steamapps", "shadercache", "440", "fozpipelines")
	writeSized(t, filepath.Join(foz, "blob.foz"), 512)

	refs := Discover(Input{GameDir: t.TempDir(), HomeDir: home, SteamAppID: "440"})
	f := byKind(refs, KindSteamFoz)
	if f == nil || f.Scope != ScopePerGame || f.SizeBytes != 512 {
		t.Errorf("foz = %+v, want per-game 512 bytes", f)
	}
	if refs := Discover(Input{GameDir: t.TempDir(), HomeDir: home, SteamAppID: ""}); byKind(refs, KindSteamFoz) != nil {
		t.Errorf("foz without appid = %+v, want none", refs)
	}
}

// Rows come out in pane order: per-game claims, precache, unclaimed,
// then globals — never in walk order.
func TestDiscoverOrdersRows(t *testing.T) {
	home := t.TempDir()
	game := t.TempDir()
	writeSized(t, filepath.Join(game, "z.dxvk-cache"), 1)
	writeSized(t, filepath.Join(game, "cache", "blob"), 2)
	writeSized(t, filepath.Join(home, ".cache", "nvidia", "GLCache", "g"), 3)
	writeSized(t, filepath.Join(home, ".cache", "mesa_shader_cache", "f"), 4)

	var kinds []Kind
	for _, r := range Discover(Input{GameDir: game, HomeDir: home}) {
		kinds = append(kinds, r.Kind)
	}
	want := []Kind{KindDXVK, KindUnclaimed, KindMesa, KindNvidia}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
}
