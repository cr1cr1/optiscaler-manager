package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// forkEnv wires a Session with a settings root (persistence assertions), a
// fork route on the fake GitHub, and a recording NewGH factory.
type forkEnv struct {
	sess      *Session
	srv       *httptest.Server
	gameRoot  string
	forkCalls []settings.Fork
}

func newForkEnv(t *testing.T) *forkEnv {
	t.Helper()
	e := &forkEnv{}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundle")
	})
	mux.HandleFunc("/repos/fork/OptiScaler-NR/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.8.92-fork","prerelease":false,"assets":[{"name":"Fork_test.7z","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "installer", "testdata", "bundle.7z"))
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
	bin := filepath.Join(e.gameRoot, "bin")
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeUIFile(t, filepath.Join(bin, "gameone.exe"), "GAME")

	cacheDir := filepath.Join(root, "cache")
	e.sess = NewSession(Deps{
		Store:        store.New(root),
		GH:           gh.NewWithBaseURL(nil, cacheDir, e.srv.URL),
		Covers:       covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:     cacheDir,
		SteamRoot:    steamRoot,
		Settings:     settings.Defaults(),
		SettingsRoot: root,
		NewGH: func(f settings.Fork) *gh.Client {
			e.forkCalls = append(e.forkCalls, f)
			return gh.NewForkWithBaseURL(nil, settings.BundleCacheDir(cacheDir, f.Slug), e.srv.URL, f.Slug, f.AssetPattern)
		},
	})
	return e
}

var nrFork = settings.Fork{Slug: "fork/OptiScaler-NR", AssetPattern: "Fork_*.7z"}

// AddFork persists; SetActiveFork swaps the GitHub client through the
// NewGH factory (resolves now hit the fork repo), clears the startup
// latest memo, and invalidates the resolved-default memo.
func TestSetActiveForkSwapsClientAndClearsMemos(t *testing.T) {
	e := newForkEnv(t)
	ctx := context.Background()

	if err := e.sess.AddFork(nrFork); err != nil {
		t.Fatalf("AddFork: %v", err)
	}
	found := false
	for _, f := range e.sess.Settings().Forks {
		if f == nrFork {
			found = true
		}
	}
	if !found {
		t.Fatalf("forks = %v, want the added fork", e.sess.Settings().Forks)
	}
	// Persisted: a fresh Load sees the fork.
	data, err := os.ReadFile(filepath.Join(e.sess.deps.SettingsRoot, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json: %v", err)
	}
	if !strings.Contains(string(data), nrFork.Slug) {
		t.Errorf("settings.json does not persist the added fork: %s", data)
	}

	// Prime the memos the fork switch must invalidate.
	e.sess.setLatestTag("v0.9.4-test")
	e.sess.refreshResolvedDefault(ctx, "latest")
	if e.sess.resolvedDefault() == "" {
		t.Fatal("resolved-default memo did not prime")
	}

	if err := e.sess.SetActiveFork(nrFork.Slug); err != nil {
		t.Fatalf("SetActiveFork: %v", err)
	}
	if got := e.sess.Settings().ActiveFork; got != nrFork.Slug {
		t.Errorf("ActiveFork = %q, want %q", got, nrFork.Slug)
	}
	if len(e.forkCalls) != 1 || e.forkCalls[0] != nrFork {
		t.Errorf("NewGH calls = %v, want exactly one with the fork", e.forkCalls)
	}
	if got := e.sess.LatestKnown(); got != "" {
		t.Errorf("LatestKnown = %q after fork switch, want cleared", got)
	}
	if got := e.sess.resolvedDefault(); got != "" {
		t.Errorf("resolvedDefault = %q after fork switch, want invalidated", got)
	}

	// The swapped client resolves against the fork repository.
	version, _, err := e.sess.ghResolveVersion(ctx, "latest")
	if err != nil {
		t.Fatalf("resolve via swapped client: %v", err)
	}
	if version != "v0.8.92-fork" {
		t.Errorf("resolved %q via swapped client, want the fork tag v0.8.92-fork", version)
	}

	// The selection persisted.
	got, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveFork != nrFork.Slug {
		t.Errorf("persisted ActiveFork = %q, want %q", got.ActiveFork, nrFork.Slug)
	}
}

// An unknown slug is refused: error, warn toast, no client swap.
func TestSetActiveForkUnknownSlugRefused(t *testing.T) {
	e := newForkEnv(t)
	if err := e.sess.SetActiveFork("ghost/repo"); err == nil {
		t.Fatal("SetActiveFork(ghost) succeeded, want refusal")
	}
	waitToast(t, e.sess, "ghost/repo")
	if got := e.sess.Settings().ActiveFork; got != settings.DefaultForkSlug {
		t.Errorf("ActiveFork = %q after refusal, want upstream", got)
	}
	if len(e.forkCalls) != 0 {
		t.Errorf("NewGH called %d times for a refused fork, want 0", len(e.forkCalls))
	}
}

// AddFork validation failures surface as an error and a warn toast.
func TestAddForkInvalidRefused(t *testing.T) {
	e := newForkEnv(t)
	if err := e.sess.AddFork(settings.Fork{Slug: "noslash", AssetPattern: "*.7z"}); err == nil {
		t.Fatal("AddFork(bad slug) succeeded, want error")
	}
	waitToast(t, e.sess, "owner/repo")
	if n := len(e.sess.Settings().Forks); n != 2 {
		t.Errorf("forks = %d after refusal, want the two built-ins", n)
	}
}

// Removing the active fork resets to upstream and swaps the client back;
// the upstream entry itself can never be removed.
func TestRemoveForkResetsActiveToUpstream(t *testing.T) {
	e := newForkEnv(t)
	if err := e.sess.AddFork(nrFork); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.SetActiveFork(nrFork.Slug); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.RemoveFork(nrFork.Slug); err != nil {
		t.Fatalf("RemoveFork: %v", err)
	}
	if got := e.sess.Settings().ActiveFork; got != settings.DefaultForkSlug {
		t.Errorf("ActiveFork = %q after removing the active fork, want upstream", got)
	}
	version, _, err := e.sess.ghResolveVersion(context.Background(), "latest")
	if err != nil {
		t.Fatalf("resolve after reset: %v", err)
	}
	if version != "v0.9.4-test" {
		t.Errorf("resolved %q after reset, want the upstream tag v0.9.4-test", version)
	}

	if err := e.sess.RemoveFork(settings.DefaultForkSlug); err == nil {
		t.Error("RemoveFork(upstream) succeeded, want refusal")
	}
}

// A committed row carries the fork it was installed from; the scan enrich
// path surfaces the manifest's fork on the row.
func TestRowCarriesForkAfterInstall(t *testing.T) {
	e := newForkEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	e.sess.Install(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	row := theRow(t, e.sess)
	if row.Fork != settings.DefaultForkSlug {
		t.Errorf("row.Fork = %q after upstream install, want %q", row.Fork, settings.DefaultForkSlug)
	}

	// A rescan keeps the fork from the manifest.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row = theRow(t, e.sess)
	if row.Fork != settings.DefaultForkSlug {
		t.Errorf("row.Fork = %q after rescan, want %q", row.Fork, settings.DefaultForkSlug)
	}
}

// ForkLabel names non-upstream installs by the fork's repo segment;
// upstream, unknown, and legacy ("") installs get no label.
func TestGameRowForkLabel(t *testing.T) {
	cases := []struct {
		fork string
		want string
	}{
		{"", ""},
		{settings.DefaultForkSlug, ""},
		{"jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", "OptiScaler-DLSSNR-PreSR-Multipass"},
		{"someone/OptiScaler-fork", "OptiScaler-fork"},
	}
	for _, tc := range cases {
		if got := (GameRow{Fork: tc.fork}).ForkLabel(); got != tc.want {
			t.Errorf("ForkLabel(%q) = %q, want %q", tc.fork, got, tc.want)
		}
	}
}
