package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// Warm boot must self-heal rows whose persisted CoverPath file vanished
// (a cache migration, a wiped cache dir): re-resolve the art in the
// background instead of showing the filler until a manual rescan (issue
// 046). Rows with live files are never re-fetched.
func TestStartHealsDeadCoverPaths(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.deps.Settings = settings.Defaults() // online lookups on
	cf := newCoverFake(t)
	coverCache := t.TempDir()
	e.sess.deps.Covers = covers.NewWithBase(cf.srv.Client(), coverCache,
		cf.srv.URL+"/steam/apps/%s/library_600x900.jpg", cf.srv.URL+"/api/storesearch/")

	dead := filepath.Join(t.TempDir(), "1091500.img") // never written
	liveDir := t.TempDir()
	live := filepath.Join(liveDir, "440.img")
	if err := os.WriteFile(live, []byte("img"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeGamesCache(t, e.sess.deps.SettingsRoot, []GameRow{{
		Title:      "Cyberpunk 2077",
		SteamAppID: "1091500",
		InstallDir: "/games/cp",
		Store:      domain.StoreManual,
		CoverPath:  dead,
	}, {
		Title:      "Half-Life 2",
		SteamAppID: "220",
		InstallDir: "/games/hl2",
		Store:      domain.StoreManual,
		CoverPath:  live,
	}})

	e.sess.Start(context.Background())

	deadline := time.Now().Add(5 * time.Second)
	for {
		row := e.sess.findRow("/games/cp")
		if row != nil && row.CoverPath != dead && strings.HasSuffix(row.CoverPath, "1091500.img") {
			if _, err := os.Stat(row.CoverPath); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			row := e.sess.findRow("/games/cp")
			t.Fatalf("dead CoverPath not healed: row=%+v cdn=%v", row, cf.cdnPaths)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The healed path is the re-fetched file in the covers cache.
	row := e.sess.findRow("/games/cp")
	if filepath.Dir(row.CoverPath) != coverCache {
		t.Errorf("healed CoverPath = %q, want a file in the covers cache %q", row.CoverPath, coverCache)
	}
	// The row with a live file was never re-fetched.
	for _, p := range cf.cdnPaths {
		if strings.Contains(p, "/440/") {
			t.Errorf("live cover re-fetched: %s", p)
		}
	}
	if r2 := e.sess.findRow("/games/hl2"); r2 == nil || r2.CoverPath != live {
		t.Errorf("live row changed: %+v", r2)
	}
	t.Logf("healed %s; cdn traffic: %v", row.CoverPath, cf.cdnPaths)
}
