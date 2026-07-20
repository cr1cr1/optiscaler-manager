package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

type appFakes struct {
	srv        *httptest.Server
	bundleHits int
	gameRoot   string
	bin        string
	st         *store.Store
	client     *gh.Client
	cacheDir   string
	fixtureSHA string
}

func newAppFakes(t *testing.T) *appFakes {
	t.Helper()
	f := &appFakes{}
	fixture, err := os.ReadFile(filepath.Join("..", "installer", "testdata", "bundle.7z"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fixture)
	f.fixtureSHA = hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, f.srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		f.bundleHits++
		_, _ = w.Write(fixture)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	root := t.TempDir()
	f.cacheDir = filepath.Join(root, "cache")
	f.st = store.New(root)
	f.client = gh.NewWithBaseURL(nil, filepath.Join(root, "ghcache"), f.srv.URL)

	f.gameRoot = t.TempDir()
	f.bin = filepath.Join(f.gameRoot, "bin")
	if err := os.MkdirAll(f.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.bin, "game.exe"), []byte("GAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// seedCache places the fixture bundle at the versioned cache path.
func (f *appFakes) seedCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(f.cacheDir, "optiscaler", "v0.9.4-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "installer", "testdata", "bundle.7z")
	dst := filepath.Join(dir, "Optiscaler_test.7z")
	if err := copyFileHelper(src, dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func copyFileHelper(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func TestInstallUsesCachedBundleFirst(t *testing.T) {
	f := newAppFakes(t)
	cached := f.seedCache(t)

	m, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if f.bundleHits != 0 {
		t.Errorf("bundle downloaded %d times despite valid cache", f.bundleHits)
	}
	if m.Resolved.SHA256 != f.fixtureSHA {
		t.Errorf("manifest sha %q != cached bundle sha %q", m.Resolved.SHA256, f.fixtureSHA)
	}
	if _, err := os.Stat(filepath.Join(f.bin, "dxgi.dll")); err != nil {
		t.Errorf("install did not complete from cache: %v", err)
	}
	t.Logf("cache hit, zero downloads, manifest sha ok (%s)", cached)
}

func TestInstallDownloadsWhenCacheMissing(t *testing.T) {
	f := newAppFakes(t)

	m, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if f.bundleHits != 1 {
		t.Fatalf("expected 1 download, got %d", f.bundleHits)
	}
	cached := filepath.Join(f.cacheDir, "optiscaler", "v0.9.4-test", "Optiscaler_test.7z")
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("downloaded bundle not cached at %s: %v", cached, err)
	}

	// Second install (after uninstall) must reuse the cached bundle even when
	// release info is served stale from the gh cooldown (consent given).
	if _, err := Uninstall(context.Background(), f.st, f.gameRoot); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	m2, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{AllowCached: true})
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if f.bundleHits != 1 {
		t.Errorf("second install re-downloaded (hits=%d)", f.bundleHits)
	}
	if m2.Resolved.SHA256 != m.Resolved.SHA256 {
		t.Errorf("sha changed between installs")
	}
	t.Log("download-once-then-cache verified")
}

// TestUninstallNotManaged: a game the store has no manifest for fails with
// the exported sentinel (errors.Is), wrapped with install-dir context,
// instead of the raw store.Load error.
func TestUninstallNotManaged(t *testing.T) {
	f := newAppFakes(t)
	_, err := Uninstall(context.Background(), f.st, f.gameRoot)
	if !errors.Is(err, ErrNotManaged) {
		t.Fatalf("Uninstall err = %v, want errors.Is(err, ErrNotManaged)", err)
	}
	if !strings.Contains(err.Error(), f.bin) {
		t.Errorf("error %q lacks install dir %q", err, f.bin)
	}
}

// TestRollbackNotManaged: Rollback maps a missing manifest to the same
// sentinel as Uninstall.
func TestRollbackNotManaged(t *testing.T) {
	f := newAppFakes(t)
	_, err := Rollback(context.Background(), f.st, f.gameRoot)
	if !errors.Is(err, ErrNotManaged) {
		t.Fatalf("Rollback err = %v, want errors.Is(err, ErrNotManaged)", err)
	}
}

// TestUninstallManagedUnaffected: a committed install uninstalls cleanly;
// once the manifest is gone, a repeat uninstall reports not-managed.
func TestUninstallManagedUnaffected(t *testing.T) {
	f := newAppFakes(t)
	f.seedCache(t)
	if _, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := Uninstall(context.Background(), f.st, f.gameRoot); err != nil {
		t.Fatalf("Uninstall of managed game: %v", err)
	}
	if _, err := Uninstall(context.Background(), f.st, f.gameRoot); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("second Uninstall err = %v, want errors.Is(err, ErrNotManaged)", err)
	}
}

var _ = domain.StatusCommitted

// --- synthetic PE fixture (mirrors internal/pever/pe_test.go) -------------

func utf16leFixture(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r > 0xFFFF {
			r = 0xFFFD
		}
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func stringInfoFixture(key, value string) []byte {
	kb := utf16leFixture(key)
	vb := utf16leFixture(value)
	valWords := len(vb)/2 + 1
	valOff := (6 + len(kb) + 2 + 3) &^ 3
	structLen := valOff + len(vb) + 2
	b := make([]byte, structLen)
	b[0], b[1] = byte(structLen), byte(structLen>>8)
	b[2], b[3] = byte(valWords), byte(valWords>>8)
	b[4], b[5] = 1, 0 // wType = text
	copy(b[6:], kb)
	copy(b[valOff:], vb)
	return b
}

// peWithProductName builds a minimal PE32+ image whose StringFileInfo
// carries the given ProductName.
func peWithProductName(name string) []byte {
	resData := stringInfoFixture("ProductName", name)
	const (
		eLfanew    = 0x40
		sectVA     = 0x1000
		sectRawOff = 0x200
		optSize    = 0xF0
	)
	b := make([]byte, sectRawOff+len(resData))
	put32 := func(off int, v uint32) {
		b[off], b[off+1], b[off+2], b[off+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	}
	b[0], b[1] = 'M', 'Z'
	b[0x3C] = eLfanew
	copy(b[eLfanew:], "PE\x00\x00")
	coff := eLfanew + 4
	b[coff+2], b[coff+3] = 1, 0 // NumberOfSections
	b[coff+16], b[coff+17] = optSize, 0
	opt := coff + 20
	b[opt], b[opt+1] = 0x0B, 0x02 // PE32+ magic
	put32(opt+112+2*8, sectVA)    // resource data-directory entry
	put32(opt+112+2*8+4, uint32(len(resData)))
	sec := opt + optSize
	copy(b[sec:], ".rsrc\x00\x00\x00")
	put32(sec+8, uint32(len(resData)))
	put32(sec+12, sectVA)
	put32(sec+16, uint32(len(resData)))
	put32(sec+20, sectRawOff)
	copy(b[sectRawOff:], resData)
	return b
}

func TestManualEntry_TitleFromExe(t *testing.T) {
	t.Run("PE ProductName wins over folder name", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "FolderName")
		exe := filepath.Join(dir, "game.exe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, peWithProductName("My Cool Game"), 0o755); err != nil {
			t.Fatal(err)
		}
		e, err := ManualEntry(dir, nil)
		if err != nil {
			t.Fatalf("ManualEntry: %v", err)
		}
		if e.Game.Name != "My Cool Game" {
			t.Errorf("Name = %q, want %q", e.Game.Name, "My Cool Game")
		}
		if e.Game.AppID != "custom_FolderName" {
			t.Errorf("AppID = %q, want folder-derived %q", e.Game.AppID, "custom_FolderName")
		}
	})

	t.Run("exe without version info keeps folder name", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "PlainFolder")
		exe := filepath.Join(dir, "game.exe")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, []byte("GAME"), 0o755); err != nil {
			t.Fatal(err)
		}
		e, err := ManualEntry(dir, nil)
		if err != nil {
			t.Fatalf("ManualEntry: %v", err)
		}
		if e.Game.Name != "PlainFolder" {
			t.Errorf("Name = %q, want %q", e.Game.Name, "PlainFolder")
		}
	})
}

// writeBrandedDLL plants a synthetic PE-branded dxgi.dll next to the game's
// exe (the injection dir for a flat manual folder is the game root itself).
func writeBrandedDLL(t *testing.T, dir string, strings map[string]string) {
	t.Helper()
	pe := testutil.StringInfoPE(false, strings, [4]uint16{0, 7, 0, 0})
	if err := os.WriteFile(filepath.Join(dir, "dxgi.dll"), pe, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newManualGameDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("GAME"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestManualEntry_DetectsExternalInstall: a manual game root carrying an
// OptiScaler-branded injection DLL (dropped in by hand, no manifest) must
// surface as external with its recovered version — manual adds bypass the
// discovery→enrich probe, so ManualEntry itself must probe.
func TestManualEntry_DetectsExternalInstall(t *testing.T) {
	dir := newManualGameDir(t, "ExternalGame")
	writeBrandedDLL(t, dir, map[string]string{
		"ProductName":      "OptiScaler",
		"OriginalFilename": "OptiScaler.dll",
	})

	e, err := ManualEntry(dir, nil)
	if err != nil {
		t.Fatalf("ManualEntry: %v", err)
	}
	if e.Status != domain.StatusExternal {
		t.Errorf("Status = %q, want %q (branded dxgi.dll undetected)", e.Status, domain.StatusExternal)
	}
	if e.OptiScalerVersion == "" {
		t.Error("OptiScalerVersion empty; want the PE FileVersion fallback")
	}
	if e.InjectionDir == "" {
		t.Error("InjectionDir empty")
	}
	t.Logf("external manual entry: status=%q version=%q", e.Status, e.OptiScalerVersion)
}

// TestManualEntry_PlainGameUnchanged: a plain game root (no injection DLL)
// keeps an empty status and version — the probe must not invent state.
func TestManualEntry_PlainGameUnchanged(t *testing.T) {
	dir := newManualGameDir(t, "PlainGame")

	e, err := ManualEntry(dir, nil)
	if err != nil {
		t.Fatalf("ManualEntry: %v", err)
	}
	if e.Status != "" {
		t.Errorf("Status = %q, want empty for a plain game", e.Status)
	}
	if e.OptiScalerVersion != "" {
		t.Errorf("OptiScalerVersion = %q, want empty", e.OptiScalerVersion)
	}
}

// TestManualEntry_DXVKNotExternal: a DXVK dxgi.dll (a different product
// brand) must not be misread as an OptiScaler install.
func TestManualEntry_DXVKNotExternal(t *testing.T) {
	dir := newManualGameDir(t, "DXVKGame")
	writeBrandedDLL(t, dir, map[string]string{
		"ProductName":      "DXVK",
		"OriginalFilename": "dxgi.dll",
	})

	e, err := ManualEntry(dir, nil)
	if err != nil {
		t.Fatalf("ManualEntry: %v", err)
	}
	if e.Status != "" {
		t.Errorf("Status = %q, want empty for a DXVK dxgi.dll", e.Status)
	}
	if e.OptiScalerVersion != "" {
		t.Errorf("OptiScalerVersion = %q, want empty", e.OptiScalerVersion)
	}
}

var optiScalerBrand = map[string]string{
	"ProductName":      "OptiScaler",
	"OriginalFilename": "OptiScaler.dll",
}

// TestManualEntry_ManagedStaysCommitted: manifest precedence — a game the
// MANAGER installed into a manual folder carries a committed manifest, so
// the branded OptiScaler PE on disk is the manager's own install and the
// entry must stay committed (never probed into external, which would make
// doUninstall refuse a legitimate uninstall).
func TestManualEntry_ManagedStaysCommitted(t *testing.T) {
	f := newAppFakes(t)
	f.seedCache(t)
	if _, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Production branding: the manager-installed OptiScaler.dll carries
	// OptiScaler PE identity (the fake bundle's does not — simulate it).
	writeBrandedDLL(t, f.bin, optiScalerBrand)

	e, err := ManualEntry(f.gameRoot, f.st)
	if err != nil {
		t.Fatalf("ManualEntry: %v", err)
	}
	if e.Status != domain.StatusCommitted {
		t.Errorf("Status = %q, want %q (committed manifest must win over the external probe)",
			e.Status, domain.StatusCommitted)
	}
	if e.ManifestID == "" {
		t.Error("ManifestID empty for a managed manual game")
	}
	t.Logf("managed manual entry: status=%q manifest=%q", e.Status, e.ManifestID)
}

// TestManualEntry_UnmanagedStillExternal: with a store that holds NO
// manifest for the directory, the probe still fires — external.
func TestManualEntry_UnmanagedStillExternal(t *testing.T) {
	dir := newManualGameDir(t, "UnmanagedGame")
	writeBrandedDLL(t, dir, optiScalerBrand)
	st := store.New(t.TempDir())

	e, err := ManualEntry(dir, st)
	if err != nil {
		t.Fatalf("ManualEntry: %v", err)
	}
	if e.Status != domain.StatusExternal {
		t.Errorf("Status = %q, want %q with an empty store", e.Status, domain.StatusExternal)
	}
	if e.OptiScalerVersion == "" {
		t.Error("OptiScalerVersion empty; want the PE FileVersion fallback")
	}
}
