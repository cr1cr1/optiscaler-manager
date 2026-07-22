package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// upgradeEnv wires a Session against a fake GitHub serving TWO releases
// (v0.10.0-test = latest, v0.9.4-test = older) and a temp Steam root with
// one game. Online lookups are on so the scan resolves the configured
// default version through the session's resolve seam; the seam is faked
// (counting calls) so scans never touch the gh client — installs still do.
type upgradeEnv struct {
	sess     *Session
	gameRoot string
	bin      string
	srv      *httptest.Server
	store    *store.Store

	resolves  atomic.Int64
	resolveFn func(requested string) (string, error)
}

func newUpgradeEnv(t *testing.T, defaultVersion string) *upgradeEnv {
	t.Helper()
	return newUpgradeEnvLookups(t, defaultVersion, true)
}

func newUpgradeEnvLookups(t *testing.T, defaultVersion string, online bool) *upgradeEnv {
	t.Helper()
	e := &upgradeEnv{}
	e.resolveFn = func(requested string) (string, error) {
		if requested == "latest" {
			return "v0.10.0-test", nil
		}
		return requested, nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.10.0-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]},{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`,
			e.srv.URL+"/bundle", e.srv.URL+"/bundle")
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
	e.bin = filepath.Join(e.gameRoot, "bin")
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeUIFile(t, filepath.Join(e.bin, "gameone.exe"), "GAME")
	writeUIFile(t, filepath.Join(e.bin, "nvngx_dlss.dll"), "DLSS")

	e.store = store.New(root)
	e.sess = NewSession(Deps{
		Store:     e.store,
		GH:        gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), e.srv.URL),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  filepath.Join(root, "cache"),
		SteamRoot: steamRoot,
		Settings:  settings.Settings{DefaultVersion: defaultVersion, OnlineLookups: online},
	})
	e.sess.resolveVersion = func(ctx context.Context, requested string) (string, bool, error) {
		e.resolves.Add(1)
		v, err := e.resolveFn(requested)
		return v, true, err
	}
	return e
}

// theRow returns the single library row or fails.
func theRow(t *testing.T, s *Session) GameRow {
	t.Helper()
	rows := s.Snapshot().Rows
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	return rows[0]
}

// TestQuickInstallCommittedUninstalls: the one-click action on a committed
// row UNINSTALLS — always, even when the resolved default is newer than the
// installed build (the setup that once produced an upgrade offer). Version
// changes belong to SwitchVersion; QuickInstall is the plain toggle.
func TestQuickInstallCommittedUninstalls(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)

	e.sess.QuickInstall(e.gameRoot)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("settle event = %q, want Uninstalled", ev.Text)
	}
	// The uninstall is the WHOLE op: no install leg may follow (the retired
	// upgrade dispatch chained uninstall-then-install here).
	drain := time.After(2 * time.Second)
	for {
		select {
		case ev := <-e.sess.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == EvOpDone {
				t.Fatalf("second settle event %q: quick action dispatched an install leg, want plain uninstall", ev.Text)
			}
		case <-drain:
			goto drained
		}
	}
drained:
	row := theRow(t, e.sess)
	if row.Status != "" {
		t.Fatalf("row status = %q after quick action, want clean (uninstalled)", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 0 {
		t.Fatalf("manifests = %d, err %v; want 0 (uninstalled)", len(manifests), err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Errorf("dxgi.dll still present after quick uninstall: err=%v", err)
	}
	t.Log("quick action on a committed row uninstalled; no upgrade dispatched")
}

// TestResolvedDefaultCachedOnce: the default version is resolved through
// the seam exactly once per distinct configured value — two scans share
// one resolution, changing Settings.DefaultVersion re-resolves once, and
// a failed resolution is NOT memoized (the next scan retries) while
// leaving the memo empty (safe offline degradation).
func TestResolvedDefaultCachedOnce(t *testing.T) {
	e := newUpgradeEnv(t, "latest")

	scanAndWait(t, e.sess)
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 1 {
		t.Fatalf("resolves after two scans = %d, want 1 (memoized)", got)
	}
	if got := e.sess.resolvedDefault(); got != "v0.10.0-test" {
		t.Fatalf("resolved default = %q, want v0.10.0-test", got)
	}

	e.sess.SetDefaultVersion("v0.9.4-test")
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 2 {
		t.Fatalf("resolves after version change = %d, want 2 (cache invalidated)", got)
	}
	if got := e.sess.resolvedDefault(); got != "v0.9.4-test" {
		t.Fatalf("resolved default after change = %q, want v0.9.4-test", got)
	}

	// Offline leg: resolution fails; the memo must not serve the stale key
	// for the NEW configured value, and the failure itself is not memoized.
	e.resolveFn = func(string) (string, error) { return "", errors.New("offline") }
	e.sess.SetDefaultVersion("v0.10.0-test")
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 3 {
		t.Fatalf("resolves after offline scan = %d, want 3", got)
	}
	if got := e.sess.resolvedDefault(); got != "" {
		t.Fatalf("resolved default offline = %q, want empty (memo unserved)", got)
	}
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 4 {
		t.Fatalf("resolves after second offline scan = %d, want 4 (failure not memoized)", got)
	}
	t.Log("default resolved once per value; failures retry; offline leaves the memo empty")
}

// installAt pins the library at an installed version: scan, quick-install,
// wait for the op to settle, and verify the committed state.
func installAt(t *testing.T, e *upgradeEnv) {
	t.Helper()
	scanAndWait(t, e.sess)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted {
		t.Fatalf("row status = %q after install, want committed", row.Status)
	}
}

// TestWarmBootResolvesDefault: a restarted app boots warm from the games
// cache and no scan runs — Start must still resolve the default version so
// the resolvedDefault memo is populated (the version dropdown reads it)
// without waiting for a manual rescan.
func TestWarmBootResolvesDefault(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	e.sess.deps.SettingsRoot = t.TempDir()
	installAt(t, e)
	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)

	// Restart: a fresh session over the same roots boots warm from cache.
	restarted := NewSession(e.sess.deps)
	restarted.resolveVersion = e.sess.resolveVersion
	restarted.Start(context.Background())
	if rows := restarted.Snapshot().Rows; len(rows) != 1 {
		t.Fatalf("warm boot rows = %d, want 1 (hydrated from cache, no scan)", len(rows))
	}

	before := e.resolves.Load()
	deadline := time.Now().Add(5 * time.Second)
	for restarted.resolvedDefault() != "v0.10.0-test" {
		if time.Now().After(deadline) {
			t.Fatalf("warm boot: resolved default = %q, want v0.10.0-test (memo populated)",
				restarted.resolvedDefault())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := e.resolves.Load() - before; got != 1 {
		t.Fatalf("resolves during warm boot = %d, want 1", got)
	}
	t.Log("warm boot resolves the default without a manual scan")
}

// TestOfflinePinnedDefaultStillResolves: with online lookups off, a pinned
// concrete default (an exact tag) needs no resolution, so the memo is
// populated without touching the resolver seam — and the provisional
// offline memo must NOT block real resolution once lookups are back on.
func TestOfflinePinnedDefaultStillResolves(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.9.4-test", false)
	installAt(t, e)

	e.sess.SetDefaultVersion("v0.10.0-test")
	scanAndWait(t, e.sess)
	if got := e.sess.resolvedDefault(); got != "v0.10.0-test" {
		t.Fatalf("offline pinned: resolved default = %q, want v0.10.0-test", got)
	}
	if got := e.resolves.Load(); got != 0 {
		t.Fatalf("resolves = %d, want 0 (offline scan must not resolve)", got)
	}

	e.sess.SetOnlineLookups(true)
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 1 {
		t.Fatalf("resolves after going back online = %d, want 1 (provisional memo re-resolved)", got)
	}
	t.Log("offline pinned default resolves without network; online re-resolves")
}

// TestOfflineLatestDefaultResolvesNothing: "latest" cannot be resolved
// without the network, so an offline scan produces no memo — and must not
// try (the resolver seam stays untouched).
func TestOfflineLatestDefaultResolvesNothing(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.9.4-test", false)
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)
	if got := e.sess.resolvedDefault(); got != "" {
		t.Fatalf("offline latest: resolved default = %q, want empty (no memo)", got)
	}
	if got := e.resolves.Load(); got != 0 {
		t.Fatalf("resolves = %d, want 0 (offline scan must not resolve)", got)
	}
	t.Log("offline latest: no memo, no resolution attempt")
}

// TestSetDefaultVersionInvalidatesMemo: changing the default version
// invalidates the resolved-default memo instantly — it is keyed by the
// configured value, so the old tag is never served for the new setting —
// and the next scan re-resolves against the new value. Re-writing the SAME
// value is a no-op: the memo survives, no re-resolution.
func TestSetDefaultVersionInvalidatesMemo(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)
	if got := e.sess.resolvedDefault(); got != "v0.10.0-test" {
		t.Fatalf("precondition: resolved default = %q, want v0.10.0-test", got)
	}

	e.sess.SetDefaultVersion("v0.9.4-test")
	if got := e.sess.resolvedDefault(); got != "" {
		t.Fatalf("resolved default after change = %q, want empty (memo keyed to the old value)", got)
	}

	scanAndWait(t, e.sess)
	if got := e.sess.resolvedDefault(); got != "v0.9.4-test" {
		t.Fatalf("resolved default after rescan = %q, want v0.9.4-test", got)
	}
	before := e.resolves.Load()
	e.sess.SetDefaultVersion("v0.9.4-test")
	scanAndWait(t, e.sess)
	if got := e.resolves.Load() - before; got != 0 {
		t.Fatalf("same-value SetDefaultVersion triggered %d re-resolves, want 0", got)
	}
	t.Log("default change invalidates the memo; same-value write keeps it")
}
