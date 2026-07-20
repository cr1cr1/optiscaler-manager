package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// TestOnlineLookups_Persists: the session toggle updates the in-memory
// settings and persists them like SetDefaultVersion.
func TestOnlineLookups_Persists(t *testing.T) {
	root := t.TempDir()
	sess := NewSession(Deps{SettingsRoot: root, Settings: settings.Defaults()})

	if !sess.Settings().OnlineLookups {
		t.Fatal("OnlineLookups = false on a fresh session, want true (default)")
	}
	sess.SetOnlineLookups(false)
	if sess.Settings().OnlineLookups {
		t.Fatal("OnlineLookups still true in memory after SetOnlineLookups(false)")
	}
	got, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings.Load: %v", err)
	}
	if got.OnlineLookups {
		t.Error("persisted OnlineLookups = true after SetOnlineLookups(false), want false")
	}

	sess.SetOnlineLookups(true)
	got, err = settings.Load(root)
	if err != nil {
		t.Fatalf("settings.Load: %v", err)
	}
	if !got.OnlineLookups {
		t.Error("persisted OnlineLookups = false after SetOnlineLookups(true), want true")
	}
	t.Log("toggle persists false and true across settings.Load")
}

// lookupFixture wires a Session against fake Steam-search and ProtonDB
// servers. The search server echoes the query title back as the result name
// (always a plausible match) with appid "777"; the summary server always
// answers tier "gold". status != 200 makes both servers fail with it.
type lookupFixture struct {
	sess       *Session
	steamHits  atomic.Int64
	protonHits atomic.Int64
}

func newLookupFixture(t *testing.T, online bool, status int) *lookupFixture {
	t.Helper()
	f := &lookupFixture{}

	steamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.steamHits.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		title, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/actions/SearchApps/"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"appid":"777","name":%q}]`, title)
	}))
	t.Cleanup(steamSrv.Close)

	protonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.protonHits.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tier":"gold","confidence":"strong","score":80,"total":100,"bestReportedTier":"gold","trendingTier":"gold"}`)
	}))
	t.Cleanup(protonSrv.Close)

	root := t.TempDir()
	f.sess = NewSession(Deps{
		Store:        store.New(root),
		CacheDir:     filepath.Join(root, "cache"),
		SteamRoot:    t.TempDir(), // empty: discovery finds no store games
		SettingsRoot: root,
		Settings:     settings.Settings{OnlineLookups: online},
		Steam:        steam.NewWithBaseURL(nil, filepath.Join(root, "steamcache"), steamSrv.URL, "test"),
		ProtonDB:     protondb.NewWithBaseURL(nil, filepath.Join(root, "protoncache"), protonSrv.URL, "test"),
	})
	return f
}

// manualDirs creates n game directories (each a 0644 .exe, so PE title
// extraction fails and the folder name is the title) and returns them.
func manualDirs(t *testing.T, n int) []string {
	t.Helper()
	var dirs []string
	for i := 0; i < n; i++ {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("FakeGame%02d", i))
		writeUIFile(t, filepath.Join(dir, "game.exe"), "GAME")
		dirs = append(dirs, dir)
	}
	return dirs
}

func scanAndWait(t *testing.T, sess *Session) State {
	t.Helper()
	sess.Scan(context.Background())
	waitEvent(t, sess, EvScanDone)
	return sess.Snapshot()
}

func TestScan_EnrichesManualRowsWhenOnline(t *testing.T) {
	f := newLookupFixture(t, true, http.StatusOK)
	f.sess.deps.Settings.ExtraDirs = manualDirs(t, 1)

	st := scanAndWait(t, f.sess)
	if len(st.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(st.Rows))
	}
	row := st.Rows[0]
	if row.SteamAppID != "777" {
		t.Errorf("SteamAppID = %q, want %q (resolved via Steam search)", row.SteamAppID, "777")
	}
	if row.ProtonTier != "gold" {
		t.Errorf("ProtonTier = %q, want %q (resolved via ProtonDB summary)", row.ProtonTier, "gold")
	}
	if f.steamHits.Load() != 1 || f.protonHits.Load() != 1 {
		t.Errorf("hits: steam=%d protondb=%d, want 1/1", f.steamHits.Load(), f.protonHits.Load())
	}
	t.Logf("manual row enriched: title=%q appid=%s tier=%s", row.Title, row.SteamAppID, row.ProtonTier)
}

func TestScan_SkipsEnrichmentWhenOffline(t *testing.T) {
	f := newLookupFixture(t, false, http.StatusOK)
	f.sess.deps.Settings.ExtraDirs = manualDirs(t, 2)

	st := scanAndWait(t, f.sess)
	if len(st.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(st.Rows))
	}
	if got := f.steamHits.Load() + f.protonHits.Load(); got != 0 {
		t.Errorf("HTTP requests with OnlineLookups=false = %d, want 0", got)
	}
	for _, r := range st.Rows {
		if r.SteamAppID != "" || r.ProtonTier != "" {
			t.Errorf("row %q enriched while offline: appid=%q tier=%q", r.Title, r.SteamAppID, r.ProtonTier)
		}
	}
	t.Log("OnlineLookups=false: scan made zero lookup HTTP requests")
}

func TestScan_EnrichmentFailureDegrades(t *testing.T) {
	f := newLookupFixture(t, true, http.StatusInternalServerError)
	f.sess.deps.Settings.ExtraDirs = manualDirs(t, 1)

	st := scanAndWait(t, f.sess) // must still settle with EvScanDone
	if len(st.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 (enrichment failure must not lose rows)", len(st.Rows))
	}
	row := st.Rows[0]
	if row.SteamAppID != "" || row.ProtonTier != "" {
		t.Errorf("row after 500s: appid=%q tier=%q, want both empty (silent degradation)", row.SteamAppID, row.ProtonTier)
	}
	t.Logf("servers 500: scan completed, row title=%q untouched", row.Title)
}

func TestScan_EnrichmentRespectsBudget(t *testing.T) {
	f := newLookupFixture(t, true, http.StatusOK)
	f.sess.deps.Settings.ExtraDirs = manualDirs(t, 12)

	st := scanAndWait(t, f.sess)
	if len(st.Rows) != 12 {
		t.Fatalf("rows = %d, want 12", len(st.Rows))
	}
	if got := f.steamHits.Load(); got > lookupBudget {
		t.Errorf("steam search calls = %d, want <= %d (per-scan lookup budget)", got, lookupBudget)
	}
	enriched := 0
	for _, r := range st.Rows {
		if r.ProtonTier != "" {
			enriched++
		}
	}
	if enriched != lookupBudget {
		t.Errorf("enriched rows = %d, want exactly %d (budget fully spent, rest skipped)", enriched, lookupBudget)
	}
	t.Logf("12 candidates: %d search calls, %d rows enriched (budget %d)", f.steamHits.Load(), enriched, lookupBudget)
}

func TestScan_EnrichesOnlyMissingAppID(t *testing.T) {
	f := newLookupFixture(t, true, http.StatusOK)

	// A steam-library game with a real numeric appid: protondb is queried
	// directly, no steam search call is needed.
	steamRoot := t.TempDir()
	gameRoot := filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeUIFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeUIFile(t, filepath.Join(gameRoot, "bin", "gameone.exe"), "GAME")
	f.sess.deps.SteamRoot = steamRoot

	st := scanAndWait(t, f.sess)
	if len(st.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(st.Rows))
	}
	row := st.Rows[0]
	if row.AppID != "100" {
		t.Fatalf("row AppID = %q, want the steam appid 100", row.AppID)
	}
	if row.SteamAppID != "100" {
		t.Errorf("SteamAppID = %q, want 100 (already known, copied over)", row.SteamAppID)
	}
	if row.ProtonTier != "gold" {
		t.Errorf("ProtonTier = %q, want gold", row.ProtonTier)
	}
	if got := f.steamHits.Load(); got != 0 {
		t.Errorf("steam search calls = %d, want 0 (numeric appid skips the search)", got)
	}
	if got := f.protonHits.Load(); got != 1 {
		t.Errorf("protondb calls = %d, want 1", got)
	}
	t.Logf("steam-library row: appid 100 -> protondb direct (tier=%s), zero search calls", row.ProtonTier)
}
