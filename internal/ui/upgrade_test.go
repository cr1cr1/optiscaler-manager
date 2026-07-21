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

	"github.com/cr1cr1/optiscaler-manager/internal/app"
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
		Settings:  settings.Settings{DefaultVersion: defaultVersion, OnlineLookups: true},
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

// TestToRowSetsUpgradeFields pins row-level eligibility: committed and
// external rows with an installed version older than the resolved default
// expose the offer; equal versions, unknown installed versions, unmanaged
// rows, and an unknown (offline) resolved default never do.
func TestToRowSetsUpgradeFields(t *testing.T) {
	entry := func(status domain.Status, installed string) app.LibraryEntry {
		return app.LibraryEntry{
			Game:              domain.Game{Name: "Game", InstallDir: "/nonexistent"},
			Status:            status,
			OptiScalerVersion: installed,
		}
	}
	sessWithDefault := func(resolved string) *Session {
		sess := NewSession(Deps{})
		sess.resolvedDefaultKey = sess.deps.Settings.DefaultVersion
		sess.resolvedDefaultVersion = resolved
		return sess
	}

	t.Run("committed older is eligible with target", func(t *testing.T) {
		row := sessWithDefault("0.10.0").toRow(context.Background(), entry(domain.StatusCommitted, "0.9.4"))
		if !row.UpgradeAvailable || row.UpgradeTarget != "0.10.0" {
			t.Errorf("row = %+v, want eligible with target 0.10.0", row)
		}
	})
	t.Run("equal versions not eligible", func(t *testing.T) {
		row := sessWithDefault("0.10.0").toRow(context.Background(), entry(domain.StatusCommitted, "0.10.0"))
		if row.UpgradeAvailable {
			t.Errorf("row = %+v, want not eligible (installed == default)", row)
		}
	})
	t.Run("external older is eligible", func(t *testing.T) {
		row := sessWithDefault("0.10.0").toRow(context.Background(), entry(domain.StatusExternal, "0.9.4"))
		if !row.UpgradeAvailable || row.UpgradeTarget != "0.10.0" {
			t.Errorf("row = %+v, want eligible with target 0.10.0", row)
		}
	})
	t.Run("not installed not eligible", func(t *testing.T) {
		row := sessWithDefault("0.10.0").toRow(context.Background(), entry(domain.Status(""), ""))
		if row.UpgradeAvailable {
			t.Errorf("row = %+v, want not eligible (never installed)", row)
		}
	})
	t.Run("unknown installed version not eligible", func(t *testing.T) {
		row := sessWithDefault("0.10.0").toRow(context.Background(), entry(domain.StatusCommitted, ""))
		if row.UpgradeAvailable {
			t.Errorf("row = %+v, want not eligible (installed version unknown)", row)
		}
	})
	t.Run("unknown resolved default degrades safe", func(t *testing.T) {
		row := sessWithDefault("").toRow(context.Background(), entry(domain.StatusCommitted, "0.9.4"))
		if row.UpgradeAvailable {
			t.Errorf("row = %+v, want not eligible (resolved default unknown, offline)", row)
		}
	})
}

// TestResolvedDefaultCachedOnce: the default version is resolved through
// the seam exactly once per distinct configured value — two scans share
// one resolution, changing Settings.DefaultVersion re-resolves once, and
// a failed resolution is NOT memoized (the next scan retries) while
// suppressing offers (safe offline degradation).
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
		t.Fatalf("resolved default offline = %q, want empty (offers suppressed)", got)
	}
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 4 {
		t.Fatalf("resolves after second offline scan = %d, want 4 (failure not memoized)", got)
	}
	t.Log("default resolved once per value; failures retry; offline suppresses offers")
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

// TestUpgradeCommittedChainsUninstallThenInstall: upgrading a committed row
// removes the old build first and only then installs the resolved default —
// the events surface in that order and the final manifest records the
// upgrade target.
func TestUpgradeCommittedChainsUninstallThenInstall(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)
	row := theRow(t, e.sess)
	if !row.UpgradeAvailable || row.UpgradeTarget != "v0.10.0-test" {
		t.Fatalf("row after retarget = %+v, want eligible for v0.10.0-test", row)
	}
	t.Logf("eligible: installed %q -> target %q", row.OptiScalerVersion, row.UpgradeTarget)

	e.sess.Upgrade(e.gameRoot)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle event = %q, want the uninstall leg first", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle event = %q, want the install leg second", ev.Text)
	}

	row = theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after upgrade = %q, want committed", row.Status)
	}
	if row.UpgradeAvailable {
		t.Errorf("row still offers an upgrade after upgrading: %+v", row)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.10.0-test" {
		t.Errorf("manifest version = %q, want v0.10.0-test", manifests[0].Resolved.Version)
	}
	t.Log("committed upgrade: uninstall settled before install; manifest at target")
}

// TestUpgradeExternalInstallsDirectly: an external row is never uninstalled
// (the manager does not own it); Upgrade goes straight to the adopt
// install, which backs the external files up SHA-verified.
func TestUpgradeExternalInstallsDirectly(t *testing.T) {
	e := newUpgradeEnv(t, "latest")
	marker := writeExternalMarker(t, e.bin)

	scanAndWait(t, e.sess)
	row := theRow(t, e.sess)
	if row.Status != domain.StatusExternal {
		t.Fatalf("row status = %q, want external", row.Status)
	}
	if !row.UpgradeAvailable || row.UpgradeTarget != "v0.10.0-test" {
		t.Fatalf("external row = %+v, want eligible for v0.10.0-test", row)
	}
	t.Logf("external version %q eligible for %q", row.OptiScalerVersion, row.UpgradeTarget)

	e.sess.Upgrade(e.gameRoot)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("settle event = %q, want a direct install (no uninstall leg)", ev.Text)
	}
	for _, toast := range e.sess.Snapshot().Toasts {
		if strings.Contains(toast.Text, "Uninstalled") || strings.Contains(toast.Text, notManagedRefusal) {
			t.Fatalf("external upgrade touched the uninstall path: %q", toast.Text)
		}
	}

	row = theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after adopt-upgrade = %q, want committed", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	// The adopt path's backup holds the exact external bytes.
	backup := filepath.Join(e.store.BackupDir(manifests[0].ID), "files", "dxgi.dll")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("external backup missing: %v", err)
	}
	if string(data) != string(marker) {
		t.Error("external backup bytes differ from the planted marker")
	}
	t.Log("external upgrade: direct adopt install, external bytes SHA-backed-up, no uninstall")
}

// TestQuickInstallDispatchesUpgradeWhenEligible: on an eligible committed
// row the one-click action must UPGRADE — not run the plain toggle, which
// would leave the game with no OptiScaler at all.
func TestQuickInstallDispatchesUpgradeWhenEligible(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)
	if row := theRow(t, e.sess); !row.UpgradeAvailable {
		t.Fatalf("row = %+v, want eligible", row)
	}

	e.sess.QuickInstall(e.gameRoot)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want upgrade chain (uninstall leg)", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want upgrade chain (install leg)", ev.Text)
	}

	// The plain toggle would have stopped after the uninstall; the upgrade
	// dispatch leaves a working new install instead.
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("dxgi.dll missing after quick action — the toggle uninstalled instead of upgrading: %v", err)
	}
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted {
		t.Fatalf("row status = %q after quick upgrade, want committed", row.Status)
	}
	t.Log("quick action on an eligible row upgraded (uninstall+install), game left installed")
}

// TestUpgradeInstallFailureRestoresBackup: when the install leg fails after
// the old build was already uninstalled, the rollback/backup-restore path
// runs and an error toast surfaces — no failed manifest, no partial files,
// no silent half-state.
func TestUpgradeInstallFailureRestoresBackup(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	e.sess.SetDefaultVersion("latest")
	scanAndWait(t, e.sess)
	if row := theRow(t, e.sess); !row.UpgradeAvailable {
		t.Fatalf("row = %+v, want eligible", row)
	}

	// Fault injection: a dangling symlink at the injection target passes the
	// uninstall leg (the file reads as vanished) but breaks the install
	// leg's first copy mid-swap, after earlier bundle files already landed.
	dxgi := filepath.Join(e.bin, "dxgi.dll")
	if err := os.Remove(dxgi); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(e.bin, "no-such-dir", "dxgi.dll"), dxgi); err != nil {
		t.Fatal(err)
	}

	e.sess.Upgrade(e.gameRoot)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg", ev.Text)
	}
	waitEvent(t, e.sess, EvOpFailed) // install leg failed mid-swap
	toast := waitToast(t, e.sess, "Failed:")
	if !toast.Warn {
		t.Errorf("failure toast Warn = false, want true: %+v", toast)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Rolled back") {
		t.Fatalf("cleanup settle = %q, want the rollback/backup-restore leg", ev.Text)
	}

	// No silent half-state: the failed manifest is rolled back, partial
	// bundle files are gone, and even the planted symlink is cleaned up.
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Status != domain.StatusRolledBack {
		t.Errorf("manifest status = %q, want rolled_back (rollback path ran)", manifests[0].Status)
	}
	if _, err := os.Lstat(dxgi); !os.IsNotExist(err) {
		t.Errorf("dxgi.dll (symlink) survived the rollback: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "fakenvapi.dll")); !os.IsNotExist(err) {
		t.Error("partial bundle file fakenvapi.dll survived the rollback")
	}
	if _, err := os.Stat(filepath.Join(e.bin, "D3D12_Optiscaler")); !os.IsNotExist(err) {
		t.Error("partial bundle dir D3D12_Optiscaler survived the rollback")
	}
	t.Log("install leg failed after uninstall: error toast surfaced, rollback cleaned the half-state")
}
