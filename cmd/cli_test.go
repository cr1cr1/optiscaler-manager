package optiscalermanager

import (
	"bytes"
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
)

func fakeSteam(t *testing.T) (steamRoot, gameRoot string) {
	t.Helper()
	steamRoot = t.TempDir()
	writeCmdTestFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeCmdTestFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	gameRoot = filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	writeCmdTestFile(t, filepath.Join(gameRoot, "bin", "gameone.exe"), "GAME")
	writeCmdTestFile(t, filepath.Join(gameRoot, "bin", "nvngx_dlss.dll"), "DLSS")
	return steamRoot, gameRoot
}

func writeCmdTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testDeps(t *testing.T, ghClient *gh.Client) (*Deps, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	root := t.TempDir()
	d := &Deps{
		Out:      out,
		ErrOut:   out,
		Store:    store.New(root),
		CacheDir: filepath.Join(root, "cache"),
		GH:       ghClient,
		Version:  "test",
	}
	return d, out
}

// fakeGitHub serves a releases list pointing at the installer fixture bundle.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "internal", "installer", "testdata", "bundle.7z"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestScanCommandListsGames(t *testing.T) {
	steamRoot, _ := fakeSteam(t)
	d, out := testDeps(t, nil)

	cmd := &ScanCmd{SteamRoot: steamRoot}
	if err := cmd.Run(d); err != nil {
		t.Fatalf("ScanCmd.Run: %v", err)
	}
	got := out.String()
	t.Logf("scan output:\n%s", got)
	for _, want := range []string{"Game One", "100", "DLSS"} {
		if !strings.Contains(got, want) {
			t.Errorf("scan output missing %q", want)
		}
	}
}

func TestInstallCommandRunsTransaction(t *testing.T) {
	srv := fakeGitHub(t)
	client := gh.NewWithBaseURL(nil, t.TempDir(), srv.URL)
	d, out := testDeps(t, client)

	_, gameRoot := fakeSteam(t)
	cmd := &InstallCmd{Path: gameRoot}
	if err := cmd.Run(d); err != nil {
		t.Fatalf("InstallCmd.Run: %v", err)
	}
	t.Logf("install output:\n%s", out)

	bin := filepath.Join(gameRoot, "bin")
	if _, err := os.Stat(filepath.Join(bin, "dxgi.dll")); err != nil {
		t.Errorf("dxgi.dll not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bin, "fakenvapi.dll")); err != nil {
		t.Errorf("fakenvapi.dll not installed: %v", err)
	}
	if !strings.Contains(out.String(), "v0.9.4-test") {
		t.Errorf("output does not report resolved version")
	}

	// The transaction is recorded and committed.
	manifests, err := d.Store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d (%v)", len(manifests), err)
	}
	if manifests[0].Status != domain.StatusCommitted {
		t.Errorf("manifest status %q, want committed", manifests[0].Status)
	}
	if manifests[0].Resolved.SHA256 == "" {
		t.Error("manifest resolved SHA256 empty")
	}

	// And uninstall reverses it through the command layer.
	ucmd := &UninstallCmd{Path: gameRoot}
	if err := ucmd.Run(d); err != nil {
		t.Fatalf("UninstallCmd.Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Error("dxgi.dll survived uninstall")
	}
}

func TestStartupRecoveryFlagsInterruptedManifests(t *testing.T) {
	d, out := testDeps(t, nil)
	m := &domain.Manifest{
		ID:           "deadbeef",
		SchemaVersion: domain.SchemaVersion,
		Status:       domain.StatusFailed,
		GameRoot:     "/games/somegame",
		InstallDir:   "/games/somegame/bin",
	}
	if err := d.Store.Save(m); err != nil {
		t.Fatal(err)
	}

	checkInterrupted(d.ErrOut, d.Store)
	got := out.String()
	t.Logf("recovery output:\n%s", got)
	if !strings.Contains(got, "/games/somegame/bin") {
		t.Errorf("warning does not name the interrupted install dir")
	}
	if !strings.Contains(got, "rollback") {
		t.Errorf("warning does not guide toward rollback")
	}
}
