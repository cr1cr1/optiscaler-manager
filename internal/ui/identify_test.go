package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// identifyFixture wires a session whose Steam client talks to a fake
// storefront handling SearchApps, appdetails, and storesearch.
type identifyFixture struct {
	sess       *Session
	appdetails map[string][2]string // appid → (name, developer)
	search     map[string]string    // normalized term → JSON items payload
	hits       atomic.Int64
}

func newIdentifyFixture(t *testing.T) *identifyFixture {
	t.Helper()
	f := &identifyFixture{appdetails: map[string][2]string{}, search: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/appdetails":
			id := r.URL.Query().Get("appids")
			nd, ok := f.appdetails[id]
			if !ok {
				fmt.Fprintf(w, `{%q:{"success":false}}`, id)
				return
			}
			fmt.Fprintf(w, `{%q:{"success":true,"data":{"name":%q,"developers":[%q]}}}`, id, nd[0], nd[1])
		case r.URL.Path == "/api/storesearch/":
			term := r.URL.Query().Get("term")
			payload, ok := f.search[strings.ToLower(term)]
			if !ok {
				_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
				return
			}
			_, _ = fmt.Fprint(w, payload)
		case strings.HasPrefix(r.URL.Path, "/actions/SearchApps/"):
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	sess := NewSession(Deps{
		SettingsRoot: t.TempDir(),
		Steam:        steam.NewWithBaseURLs(srv.Client(), t.TempDir(), srv.URL, srv.URL, "0.8.0"),
	})
	f.sess = sess
	return f
}

// A row whose appid came from steam_appid.txt upgrades its tail-chain
// title to the canonical store name during enrich.
func TestIdentify_AppIDUpgradesToCanonical(t *testing.T) {
	f := newIdentifyFixture(t)
	f.appdetails["2322010"] = [2]string{"God of War Ragnarök", "Santa Monica Studio"}
	row := GameRow{Title: "GoWR", InstallDir: "/games/God of War Ragnarok", Store: domain.StoreManual, SteamAppID: "2322010", TitleSource: "pe"}

	if !f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam) {
		t.Fatal("live = false, want a live appdetails call")
	}
	if row.Title != "God of War Ragnarök" || row.TitleSource != "storeid" {
		t.Errorf("row = %+v, want canonical title with storeid source", row)
	}
}

// A codename row without an appid resolves through the normalized store
// search: the codename itself finds nothing, the folder name matches.
func TestIdentify_FuzzyResolvesCanonical(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["black myth wukong"] = `{"total":1,"items":[{"type":"app","name":"Black Myth: Wukong","id":2358720,"platforms":{"windows":true}}]}`
	row := GameRow{Title: "b1", InstallDir: "/games/Black Myth Wukong", Store: domain.StoreManual, TitleSource: "pe"}

	if !f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam) {
		t.Fatal("live = false, want a live storesearch call")
	}
	if row.Title != "Black Myth: Wukong" || row.SteamAppID != "2358720" || row.TitleSource != "fuzzy" {
		t.Errorf("row = %+v, want fuzzy canonical title + appid", row)
	}
}

// Traps: unrelated store answers never rename the row.
func TestIdentify_FuzzyRejectsTraps(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["doom"] = `{"total":2,"items":[{"type":"app","name":"Doom 3","id":1,"platforms":{"windows":true}},{"type":"app","name":"Doom Eternal","id":2,"platforms":{"windows":true}}]}`
	row := GameRow{Title: "Doom", InstallDir: "/games/Doom", Store: domain.StoreManual, TitleSource: "folder"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "Doom" || row.SteamAppID != "" {
		t.Errorf("row = %+v, want unchanged (short-title trap)", row)
	}
}

// A 75-89 score needs corroboration: developer must match the PE
// CompanyName.
func TestIdentify_FuzzyCorroboration(t *testing.T) {
	f := newIdentifyFixture(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	pe := testutil.StringInfoPE(false, map[string]string{"ProductName": "X", "CompanyName": "Santa Monica Studio"}, [4]uint16{1, 0, 0, 0})
	writeUIFile(t, exe, string(pe))
	// Exact normalized match with an edition mismatch and no platform
	// bonus scores 80: it only lands when the developer corroborates the
	// PE CompanyName.
	f.search["god of war ascension"] = `{"total":1,"items":[{"type":"app","name":"God of War: Ascension Special Edition","id":44,"platforms":{"windows":false}}]}`
	f.appdetails["44"] = [2]string{"God of War: Ascension Special Edition", "Santa Monica Studio"}
	row := GameRow{Title: "god of war ascension", InstallDir: dir, Store: domain.StoreManual, ExePath: exe, TitleSource: "pe"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "God of War: Ascension Special Edition" || row.SteamAppID != "44" {
		t.Errorf("corroborated: row = %+v", row)
	}
}

func TestIdentify_FuzzyCorroborationMismatch(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["god of war ascension"] = `{"total":1,"items":[{"type":"app","name":"God of War: Ascension Special Edition","id":45,"platforms":{"windows":false}}]}`
	f.appdetails["45"] = [2]string{"God of War: Ascension Special Edition", "Someone Else"}
	row := GameRow{Title: "god of war ascension", InstallDir: "/games/x", Store: domain.StoreManual, TitleSource: "pe"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "god of war ascension" || row.SteamAppID != "" {
		t.Errorf("uncorroborated: row = %+v, want unchanged", row)
	}
}

// User-pinned rows are frozen: no identification calls at all.
func TestIdentify_OverrideFrozen(t *testing.T) {
	f := newIdentifyFixture(t)
	row := GameRow{Title: "Pinned", InstallDir: "/games/x", Store: domain.StoreManual, SteamAppID: "1", TitleSource: "override"}
	if f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam) {
		t.Fatal("live = true for an override row")
	}
	if row.Title != "Pinned" || f.hits.Load() != 0 {
		t.Errorf("row = %+v hits = %d, want untouched and zero calls", row, f.hits.Load())
	}
}

// Store rows are never re-identified.
func TestIdentify_StoreRowsSkipped(t *testing.T) {
	f := newIdentifyFixture(t)
	row := GameRow{Title: "Steam Game", InstallDir: "/games/x", Store: domain.StoreSteam, SteamAppID: "220"}
	if f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam) {
		t.Fatal("live = true for a steam row")
	}
	if row.Title != "Steam Game" || f.hits.Load() != 0 {
		t.Errorf("row = %+v hits = %d, want untouched and zero calls", row, f.hits.Load())
	}
}

// A codename short enough to exact-match an unrelated store item ("b1" →
// "B1") is too ambiguous to query: the fuzzy step skips it and the folder
// candidate decides instead.
func TestIdentify_ShortCandidateSkipped(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["b1"] = `{"total":1,"items":[{"type":"app","name":"B1","id":1,"platforms":{"windows":true}}]}`
	f.search["black myth wukong"] = `{"total":1,"items":[{"type":"app","name":"Black Myth: Wukong","id":2358720,"platforms":{"windows":true}}]}`
	row := GameRow{Title: "b1", InstallDir: "/games/Black Myth Wukong", Store: domain.StoreManual, TitleSource: "pe"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "Black Myth: Wukong" || row.SteamAppID != "2358720" {
		t.Errorf("row = %+v, want the folder-driven match, not the B1 trap", row)
	}
}

// A hard Steam appid outranks in-dir metadata: a goggame row with a
// detected appid upgrades to the canonical store name, while a goggame
// row without one keeps its metadata title and never goes fuzzy.
func TestIdentify_AppIDOutranksInDirMetadata(t *testing.T) {
	f := newIdentifyFixture(t)
	f.appdetails["1281630"] = [2]string{"Anno 1404 - History Edition", "Ubisoft"}

	withID := GameRow{Title: "Anno 1404", InstallDir: "/games/Anno 1404 Gold Edition", Store: domain.StoreManual, SteamAppID: "1281630", TitleSource: "goggame"}
	f.sess.identifyRow(context.Background(), &withID, f.sess.deps.Steam)
	if withID.Title != "Anno 1404 - History Edition" || withID.TitleSource != "storeid" {
		t.Errorf("withID row = %+v, want canonical upgrade over goggame", withID)
	}

	hitsBefore := f.hits.Load()
	noID := GameRow{Title: "Anno 1404", InstallDir: "/games/Anno 1404 Gold Edition", Store: domain.StoreManual, TitleSource: "goggame"}
	f.sess.identifyRow(context.Background(), &noID, f.sess.deps.Steam)
	if noID.Title != "Anno 1404" || f.hits.Load() != hitsBefore {
		t.Errorf("noID row = %+v (+%d calls), want metadata title kept, zero fuzzy", noID, f.hits.Load()-hitsBefore)
	}
}

// A hostile cached SteamAppID never reaches the store endpoint (cache
// filename traversal guard): non-numeric appids are refused before any call.
func TestIdentify_RejectsNonNumericAppID(t *testing.T) {
	f := newIdentifyFixture(t)
	row := GameRow{Title: "X", InstallDir: "/games/x", Store: domain.StoreManual, SteamAppID: "x/../../evil", TitleSource: "pe"}
	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if f.hits.Load() != 0 {
		t.Errorf("hits = %d, want zero calls for a hostile appid", f.hits.Load())
	}
}

// A game that only exists as a DLC store page (The Talos Principle 2:
// Road to Elysium) resolves when no base app matches: dlc items are
// scored after apps, never instead of them.
func TestIdentify_DLCItemResolvesWhenNoAppMatches(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["the talos principle 2 road to elysium"] = `{"total":1,"items":[{"type":"dlc","name":"The Talos Principle 2: Road to Elysium","id":101,"platforms":{"windows":true}}]}`
	row := GameRow{Title: "Talos2", InstallDir: "/games/The Talos Principle 2 Road to Elysium", Store: domain.StoreManual, TitleSource: "pe"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "The Talos Principle 2: Road to Elysium" || row.SteamAppID != "101" || row.TitleSource != "fuzzy" {
		t.Errorf("row = %+v, want the dlc store page title", row)
	}
}

// An accepted base app still beats any dlc item.
func TestIdentify_AppBeatsDLC(t *testing.T) {
	f := newIdentifyFixture(t)
	f.search["some game"] = `{"total":2,"items":[{"type":"app","name":"Some Game","id":50,"platforms":{"windows":true}},{"type":"dlc","name":"Some Game","id":51,"platforms":{"windows":true}}]}`
	row := GameRow{Title: "Some Game", InstallDir: "/games/x", Store: domain.StoreManual, TitleSource: "pe"}
	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.SteamAppID != "50" {
		t.Errorf("appid = %q, want the app item (50), not the dlc (51)", row.SteamAppID)
	}
}

// Appid-bearing rows jump the enrichment queue: the cheap, high-value
// canonical upgrades must not starve behind a library of fuzzy
// candidates under the live budget.
func TestEnrichOnline_PrioritizesAppIDRows(t *testing.T) {
	f := newIdentifyFixture(t)
	pdb := newLookupFixture(t, true, http.StatusOK)
	f.sess.deps.ProtonDB = pdb.sess.deps.ProtonDB
	for _, id := range []string{"2322010", "1677280", "1693980"} {
		f.appdetails[id] = [2]string{"Canonical " + id, "Dev"}
	}
	rows := make([]GameRow, 0, 12)
	for i := 0; i < 9; i++ {
		rows = append(rows, GameRow{Title: fmt.Sprintf("Obscure%d", i), InstallDir: fmt.Sprintf("/games/obscure%d", i), Store: domain.StoreManual, TitleSource: "pe"})
	}
	for _, id := range []string{"2322010", "1677280", "1693980"} {
		rows = append(rows, GameRow{Title: "Codename", InstallDir: "/games/" + id, Store: domain.StoreManual, SteamAppID: id, TitleSource: "pe"})
	}

	snap := settings.Defaults()
	f.sess.enrichOnline(context.Background(), rows, snap)
	for _, r := range rows {
		if r.SteamAppID != "" && r.TitleSource != "storeid" {
			t.Errorf("appid row %s NOT upgraded (src=%q) — prioritization failed", r.SteamAppID, r.TitleSource)
		}
	}
}

// PCGamingWiki is the secondary source: when Steam's storesearch finds
// nothing (GOG/off-store games), the wiki's canonical page title wins.
func TestIdentify_PCGWFallbackResolves(t *testing.T) {
	f := newIdentifyFixture(t)
	wikiSrv := newPCGWStub(t, "A GOG Only Title")
	f.sess.deps.PCGW = pcgw.NewWithBaseURL(wikiSrv.Client(), t.TempDir(), wikiSrv.URL, "test")
	row := GameRow{Title: "A GOG Only Title", InstallDir: "/games/A GOG Only Title", Store: domain.StoreManual, TitleSource: "pe"}

	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "A GOG Only Title" || row.TitleSource != "fuzzy" {
		t.Errorf("row = %+v, want the wiki title", row)
	}
}

// A wiki miss leaves the row alone.
func TestIdentify_PCGWMissKeepsRow(t *testing.T) {
	f := newIdentifyFixture(t)
	wikiSrv := newPCGWStub(t, "")
	f.sess.deps.PCGW = pcgw.NewWithBaseURL(wikiSrv.Client(), t.TempDir(), wikiSrv.URL, "test")
	row := GameRow{Title: "zzz nothing", InstallDir: "/games/zzz", Store: domain.StoreManual, TitleSource: "pe"}
	f.sess.identifyRow(context.Background(), &row, f.sess.deps.Steam)
	if row.Title != "zzz nothing" {
		t.Errorf("row = %+v, want unchanged", row)
	}
}
