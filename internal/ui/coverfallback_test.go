package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// coverFake records CDN and store-search traffic, serving art for one
// known appid.
type coverFake struct {
	srv                   *httptest.Server
	cdnPaths              []string
	searches              []string
	knownAppID, knownName string
}

func newCoverFake(t *testing.T) *coverFake {
	t.Helper()
	f := &coverFake{knownAppID: "1091500", knownName: "Cyberpunk 2077"}
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		f.cdnPaths = append(f.cdnPaths, r.URL.Path)
		if !strings.Contains(r.URL.Path, f.knownAppID) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = fmt.Fprint(w, "img")
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		f.searches = append(f.searches, term)
		if !strings.Contains(strings.ToLower(f.knownName), strings.ToLower(term)) {
			_, _ = fmt.Fprint(w, `{"items":[]}`)
			return
		}
		fmt.Fprintf(w, `{"items":[{"id":%s,"name":%q,"platforms":{"windows":true}}]}`, f.knownAppID, f.knownName)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *coverFake) covers(t *testing.T) *covers.Covers {
	t.Helper()
	return covers.NewWithBase(f.srv.Client(), t.TempDir(),
		f.srv.URL+"/steam/apps/%s/library_600x900.jpg", f.srv.URL+"/api/storesearch/")
}

// A manual game's "custom_<folder>" id is not a Steam appid: digits in the
// folder name must never become a CDN request (wrong art for a wrong appid,
// plus a bogus .miss key that suppresses the real search for a week).
func TestToRowManualGameSkipsBogusAppID(t *testing.T) {
	f := newCoverFake(t)
	s := NewSession(Deps{Covers: f.covers(t)})

	s.toRow(context.Background(), app.LibraryEntry{Game: domain.Game{
		AppID:      "custom_Hades2",
		Name:       "Hades2",
		Store:      domain.StoreManual,
		InstallDir: t.TempDir(),
	}})

	for _, p := range f.cdnPaths {
		if strings.Contains(p, "/apps/2/") {
			t.Fatalf("CDN hit for the folder-derived appid: %s", p)
		}
	}
	t.Logf("cdn paths: %v, searches: %v", f.cdnPaths, f.searches)
}

// A manual row whose identification resolved a canonical title but no
// appid keeps the raw-title search's placeholder. refreshCovers must retry
// with the resolved title — the actual best-probability query.
func TestRefreshCoversRetriesByResolvedTitle(t *testing.T) {
	f := newCoverFake(t)
	s := NewSession(Deps{Covers: f.covers(t)})

	rows := []GameRow{{
		Title:      "Cyberpunk 2077", // canonical, resolved by identifyRow
		InstallDir: "/games/cp",
		Store:      domain.StoreManual,
		CoverPath:  "/nonexistent/_placeholder.png",
	}}
	s.refreshCovers(context.Background(), rows)

	if !strings.HasSuffix(rows[0].CoverPath, f.knownAppID+".img") {
		t.Errorf("CoverPath = %q, want art found via the resolved title", rows[0].CoverPath)
	}
	if len(f.searches) == 0 || f.searches[len(f.searches)-1] != "Cyberpunk 2077" {
		t.Errorf("searches = %v, want the resolved title queried", f.searches)
	}

	// A row that already has real art (.img) is not re-searched.
	f.searches = nil
	rows[0].CoverPath = "/cache/" + f.knownAppID + ".img"
	rows[0].SteamAppID = ""
	s.refreshCovers(context.Background(), rows)
	if len(f.searches) != 0 {
		t.Errorf("re-searched a row that already has art: %v", f.searches)
	}
	t.Log("resolved-title retry fired once, art rows skipped")
}

// A manually added game must get the same online enrichment a scan would
// give it — canonical title, appid, and the cover rebind — in the add's
// own async pass, not only after the user rescans by hand.
func TestAddDirectoryEnrichesManualGame(t *testing.T) {
	f := newIdentifyFixture(t)
	f.sess.deps.Settings = settings.Defaults() // fixture zero value has OnlineLookups off
	f.search["cyberpunk2077"] = `{"total":1,"items":[{"id":1091500,"name":"Cyberpunk 2077","type":"app","platforms":{"windows":true}}]}`
	cf := newCoverFake(t)
	f.sess.deps.Covers = cf.covers(t)

	dir := t.TempDir()
	writeUIFile(t, filepath.Join(dir, "cyberpunk2077.exe"), "MZGAME")

	f.sess.AddDirectory(dir)
	waitEvent(t, f.sess, EvScanDone) // "directory added"

	row := f.sess.findRow(canonicalDir(dir))
	if row == nil {
		t.Fatal("added row missing")
	}
	if row.Title != "Cyberpunk 2077" || row.SteamAppID != "1091500" {
		t.Errorf("row = title %q appid %q, want the identified canonical pair", row.Title, row.SteamAppID)
	}
	if !strings.HasSuffix(row.CoverPath, "1091500.img") {
		t.Errorf("CoverPath = %q, want art for the identified appid", row.CoverPath)
	}
	t.Logf("manual add enriched: %q (%s), cover %s", row.Title, row.SteamAppID, row.CoverPath)
}

