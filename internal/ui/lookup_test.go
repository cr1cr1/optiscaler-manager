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

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
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

// TestScan_EnrichmentBudgetCountsLiveRequestsOnly: the lookup budget bounds
// rows that required a live request, not processed rows — cache hits are
// free. Six rows warmed by a first scan must not eat the budget: a second
// scan with 8 more candidates still enriches all 8 live rows.
func TestScan_EnrichmentBudgetCountsLiveRequestsOnly(t *testing.T) {
	f := newLookupFixture(t, true, http.StatusOK)
	cached := manualDirs(t, 6)
	f.sess.deps.Settings.ExtraDirs = cached
	scanAndWait(t, f.sess) // warms the steam+protondb caches for 6 rows
	if got := f.steamHits.Load(); got != 6 {
		t.Fatalf("warm-up scan: steam hits = %d, want 6", got)
	}

	// Distinct titles: the row title is the folder name, so a reused
	// FakeGameNN name would hit the warm steam cache instead of going live.
	extra := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("LiveGame%02d", i))
		writeUIFile(t, filepath.Join(dir, "game.exe"), "GAME")
		extra = append(extra, dir)
	}
	f.sess.deps.Settings.ExtraDirs = append(cached, extra...)
	st := scanAndWait(t, f.sess)
	if len(st.Rows) != 14 {
		t.Fatalf("rows = %d, want 14", len(st.Rows))
	}
	enriched := 0
	for _, r := range st.Rows {
		if r.ProtonTier != "" {
			enriched++
		}
	}
	if enriched != 14 {
		t.Errorf("enriched rows = %d, want 14 (6 cached + 8 live, cache hits are budget-free)", enriched)
	}
	if got := f.steamHits.Load(); got != 14 {
		t.Errorf("steam search calls = %d, want 14 (6 warm-up + 8 live; cached rows re-request nothing)", got)
	}
	t.Logf("14 candidates, 6 cached: %d live search calls, %d rows enriched", f.steamHits.Load(), enriched)
}

// TestScan_EnrichmentPermanentFailuresDontStarve: rows that fail forever
// (no Steam match) must not starve later rows: once the failures are
// negative-cached they stop consuming the budget, and a valid row behind 8
// permanent failures is enriched by the second scan at the latest.
func TestScan_EnrichmentPermanentFailuresDontStarve(t *testing.T) {
	var steamHits, protonHits atomic.Int64
	steamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		steamHits.Add(1)
		title, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/actions/SearchApps/"))
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(title, "08") { // only the 9th title resolves
			fmt.Fprintf(w, `[{"appid":"777","name":%q}]`, title)
			return
		}
		_, _ = fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(steamSrv.Close)
	protonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protonHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tier":"gold","confidence":"strong","score":80,"total":100,"bestReportedTier":"gold","trendingTier":"gold"}`)
	}))
	t.Cleanup(protonSrv.Close)

	root := t.TempDir()
	sess := NewSession(Deps{
		Store:        store.New(root),
		CacheDir:     filepath.Join(root, "cache"),
		SteamRoot:    t.TempDir(),
		SettingsRoot: root,
		Settings:     settings.Settings{OnlineLookups: true},
		Steam:        steam.NewWithBaseURL(nil, filepath.Join(root, "steamcache"), steamSrv.URL, "test"),
		ProtonDB:     protondb.NewWithBaseURL(nil, filepath.Join(root, "protoncache"), protonSrv.URL, "test"),
	})
	sess.deps.Settings.ExtraDirs = manualDirs(t, 9)

	tierOf := func(st State, suffix string) string {
		t.Helper()
		for _, r := range st.Rows {
			if strings.HasSuffix(r.InstallDir, suffix) {
				return r.ProtonTier
			}
		}
		t.Fatalf("no row with InstallDir suffix %q", suffix)
		return ""
	}

	st := scanAndWait(t, sess) // scan 1: budget spent on the 8 failures
	if got := tierOf(st, "FakeGame08"); got != "" {
		t.Fatalf("scan 1: row 9 tier = %q, want empty (budget went to the 8 live failures)", got)
	}
	if got := steamHits.Load(); got != lookupBudget {
		t.Fatalf("scan 1: steam hits = %d, want %d (8 live failing searches)", got, lookupBudget)
	}

	st = scanAndWait(t, sess) // scan 2: negatives cached, row 9 resolves
	if got := tierOf(st, "FakeGame08"); got != "gold" {
		t.Errorf("scan 2: row 9 tier = %q, want gold (cached negatives freed the budget)", got)
	}
	if got := steamHits.Load(); got != lookupBudget+1 {
		t.Errorf("steam hits after scan 2 = %d, want %d (failures served from the negative cache)", got, lookupBudget+1)
	}
	t.Logf("row 9 enriched after 2 scans; steam hits %d", steamHits.Load())
}

// TestEnrichRow_SkipsNonNumericAppID: an appid resolved from a Steam search
// is attacker-controlled input to the ProtonDB URL path; anything that is
// not a bare numeric appid is skipped before the ProtonDB call.
func TestEnrichRow_SkipsNonNumericAppID(t *testing.T) {
	var protonHits atomic.Int64
	steamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/actions/SearchApps/"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"appid":"../evil","name":%q}]`, title)
	}))
	t.Cleanup(steamSrv.Close)
	protonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protonHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tier":"gold"}`)
	}))
	t.Cleanup(protonSrv.Close)

	st := steam.NewWithBaseURL(nil, t.TempDir(), steamSrv.URL, "test")
	pdb := protondb.NewWithBaseURL(nil, t.TempDir(), protonSrv.URL, "test")
	sess := NewSession(Deps{})
	row := GameRow{Title: "Evil Game", AppID: "custom_evil", Store: domain.StoreManual}

	sess.enrichRow(context.Background(), &row, st, pdb)
	if got := protonHits.Load(); got != 0 {
		t.Errorf("protondb calls = %d, want 0 (non-numeric appid %q must be skipped)", got, "../evil")
	}
	if row.SteamAppID != "" || row.ProtonTier != "" {
		t.Errorf("row mutated by a rejected appid: %+v", row)
	}
	t.Log("non-numeric appid skipped: no protondb call, row untouched")
}
