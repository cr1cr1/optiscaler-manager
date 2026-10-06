package ui

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// nrZipFork publishes zips shaped like the real DLSSNR distribution:
// injector at the archive root, support DLLs under an OptiScaler/ subdir,
// no fakenvapi.
var nrZipFork = settings.Fork{Slug: "fork/OptiScaler-NR", AssetPattern: "Fork_*.zip"}

func writeUIZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func nrZipShaped() map[string]string {
	return map[string]string{
		"OptiScaler.dll":         "NR-INJECTOR",
		"OptiScaler.ini":         "NR-INI",
		"OptiScaler/libxess.dll": "NR-XESS",
		"OptiScaler/libxell.dll": "NR-XELL",
		"docs/NR-VULKAN.md":      "NR-DOC",
		"setup_linux.sh":         "NR-SETUP",
	}
}

// forkSwitchEnv wires a Session whose fake GitHub serves the upstream 7z
// AND a DLSSNR-shaped zip for the NR fork, with a recording NewGH factory.
type forkSwitchEnv struct {
	sess     *Session
	gameRoot string
	bin      string
	srv      *httptest.Server
	store    *store.Store
}

func newForkSwitchEnv(t *testing.T) *forkSwitchEnv {
	t.Helper()
	e := &forkSwitchEnv{}
	forkZip := filepath.Join(t.TempDir(), "Fork_test.zip")
	writeUIZip(t, forkZip, nrZipShaped())

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundle7z")
	})
	mux.HandleFunc("/repos/fork/OptiScaler-NR/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.8.92-fork","prerelease":false,"assets":[{"name":"Fork_test.zip","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundlezip")
	})
	mux.HandleFunc("/bundle7z", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "installer", "testdata", "bundle.7z"))
	})
	mux.HandleFunc("/bundlezip", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, forkZip)
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)

	root := t.TempDir()
	steamRoot := t.TempDir()
	e.gameRoot = filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	e.bin = filepath.Join(e.gameRoot, "bin")
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeUIFile(t, filepath.Join(e.bin, "gameone.exe"), "GAME")

	cacheDir := filepath.Join(root, "cache")
	e.store = store.New(root)
	e.sess = NewSession(Deps{
		Store:     e.store,
		GH:        gh.NewWithBaseURL(nil, cacheDir, e.srv.URL),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  cacheDir,
		SteamRoot: steamRoot,
		Settings:  settings.Defaults(),
		NewGH: func(f settings.Fork) *gh.Client {
			return gh.NewForkWithBaseURL(nil, settings.BundleCacheDir(cacheDir, f.Slug), e.srv.URL, f.Slug, f.AssetPattern)
		},
	})
	return e
}

// Switching a committed game to a release of a DIFFERENT fork moves the old
// distribution's whole file set into <repo>.YYMMDD inside the injection
// dir (named after the OLD fork, nested paths preserved) instead of
// silently deleting it, then installs the new distribution.
func TestForkSwitchRelocatesOldForkFiles(t *testing.T) {
	e := newForkSwitchEnv(t)
	if err := e.sess.AddFork(nrZipFork); err != nil {
		t.Fatalf("AddFork: %v", err)
	}
	if err := e.sess.SetActiveFork(nrZipFork.Slug); err != nil {
		t.Fatalf("SetActiveFork(nr): %v", err)
	}
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.Install(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	row := theRow(t, e.sess)
	if row.Status != domain.StatusCommitted || row.Fork != nrZipFork.Slug {
		t.Fatalf("row after NR install = %q/%q, want committed %q", row.Status, row.Fork, nrZipFork.Slug)
	}
	if data, err := os.ReadFile(filepath.Join(e.bin, "dxgi.dll")); err != nil || string(data) != "NR-INJECTOR" {
		t.Fatalf("NR dxgi.dll = %q (err %v), want NR-INJECTOR", data, err)
	}

	if err := e.sess.SetActiveFork(settings.DefaultForkSlug); err != nil {
		t.Fatalf("SetActiveFork(upstream): %v", err)
	}
	e.sess.now = func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local) }

	e.sess.SwitchVersion(e.gameRoot, "v0.9.4-test")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg first", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want the install leg second", ev.Text)
	}

	// The old NR distribution sits in the dated dir — named after the OLD
	// fork's repo segment — bytes and nested layout intact.
	dated := filepath.Join(e.bin, "OptiScaler-NR.261007")
	data, err := os.ReadFile(filepath.Join(dated, "dxgi.dll"))
	if err != nil {
		t.Fatalf("old fork's dxgi.dll not relocated to %s: %v", dated, err)
	}
	if string(data) != "NR-INJECTOR" {
		t.Errorf("relocated dxgi.dll = %q, want NR-INJECTOR", data)
	}
	if data, err := os.ReadFile(filepath.Join(dated, "OptiScaler", "libxess.dll")); err != nil || string(data) != "NR-XESS" {
		t.Errorf("relocated nested OptiScaler/libxess.dll = %q (err %v), want NR-XESS", data, err)
	}
	// The ini is NOT in the dated dir: the switch chain preserved it into
	// the new install instead.
	if _, err := os.Stat(filepath.Join(dated, "OptiScaler.ini")); !os.IsNotExist(err) {
		t.Error("OptiScaler.ini landed in the dated dir; it belongs to the new install")
	}

	// The upstream distribution stands installed.
	if _, err := os.Stat(filepath.Join(e.bin, "fakenvapi.dll")); err != nil {
		t.Errorf("upstream fakenvapi.dll missing after switch: %v", err)
	}
	row = theRow(t, e.sess)
	if row.Status != domain.StatusCommitted || row.Fork != settings.DefaultForkSlug {
		t.Errorf("row after fork switch = %q/%q, want committed upstream", row.Status, row.Fork)
	}
	if row.OptiScalerVersion != "v0.9.4-test" {
		t.Errorf("row version after fork switch = %q, want v0.9.4-test", row.OptiScalerVersion)
	}
}

// A same-fork release switch keeps using the internal SHA-verified backup:
// no dated dir appears in the game folder.
func TestSameForkSwitchLeavesNoDatedDir(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)
	e.sess.now = func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local) }

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	waitEvent(t, e.sess, EvOpDone)
	waitEvent(t, e.sess, EvOpDone)

	entries, err := os.ReadDir(e.bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range entries {
		if en.IsDir() && strings.HasPrefix(en.Name(), "OptiScaler.") {
			t.Errorf("dated backup dir %q left behind by a SAME-fork switch", en.Name())
		}
	}
	if row := theRow(t, e.sess); row.OptiScalerVersion != "v0.10.0-test" {
		t.Errorf("row version = %q, want v0.10.0-test", row.OptiScalerVersion)
	}
}
