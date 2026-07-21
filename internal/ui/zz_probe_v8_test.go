package ui

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
)

func waitProbeEvent(t *testing.T, s *Session, kind EventKind) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case ev := <-s.Events():
			if ev.Kind == kind {
				return
			}
		case <-time.After(deadline.Sub(time.Now())):
			t.Fatalf("timed out waiting for %v", kind)
		}
	}
	t.Fatalf("timed out waiting for %v", kind)
}

// Scratch v0.8 real-library probe (deleted after verification): runs the
// full session scan against the user's actual roots with LIVE Steam
// endpoints and checks every identification rule on real data.
func TestZZProbeV8RealLibrary(t *testing.T) {
	if os.Getenv("OM_V8_PROBE") == "" {
		t.Skip("set OM_V8_PROBE=1 to run the live real-library probe")
	}
	sess := NewSession(Deps{
		SettingsRoot: t.TempDir(),
		Settings: settings.Settings{
			ExtraDirs:     []string{"/mnt/linux2/Games", "/mnt/linux3/Games"},
			OnlineLookups: true,
		},
		Steam:    steam.NewWithBaseURL(&http.Client{}, t.TempDir(), "https://steamcommunity.com", "probe-0.8"),
		ProtonDB: protondb.New(&http.Client{}, t.TempDir(), "probe-0.8"),
	})
	// The lookup budget resolves 8 rows per scan; rescan until the
	// library stops changing (caches make earlier rows free).
	for i := 0; i < 4; i++ {
		sess.Scan(context.Background())
		waitProbeEvent(t, sess, EvScanDone)
	}
	rows := sess.Snapshot().Rows
	t.Logf("rows: %d", len(rows))
	byDir := map[string]GameRow{}
	for _, r := range rows {
		byDir[r.InstallDir] = r
		t.Logf("row: %q src=%q appid=%q dir=%q", r.Title, r.TitleSource, r.SteamAppID, r.InstallDir)
	}
	expect := []struct {
		dir, title, src, appid string
	}{
		{"/mnt/linux2/Games/God of War Ragnarok", "God of War Ragnarök", "storeid", "2322010"},
		{"/mnt/linux3/Games/Company of Heroes 3", "Company of Heroes 3", "storeid", "1677280"},
		{"/mnt/linux3/Games/Dead Space Remake", "Dead Space", "storeid", "1693980"},
		{"/mnt/linux2/Games/A Plague Tale Requiem", "A Plague Tale: Requiem", "goggame", ""},
		{"/mnt/linux3/Games/PREY", "Prey", "egstore", ""},
	}
	for _, e := range expect {
		r, ok := byDir[e.dir]
		if !ok {
			t.Errorf("missing row %s", e.dir)
			continue
		}
		if r.Title != e.title || r.TitleSource != e.src || (e.appid != "" && r.SteamAppID != e.appid) {
			t.Errorf("%s: title=%q src=%q appid=%q, want %q/%q/%q", e.dir, r.Title, r.TitleSource, r.SteamAppID, e.title, e.src, e.appid)
		}
	}
	unityDirs := []string{"Tails of Iron", "Tails of Iron Bright Fir Forest", "STASIS BONE TOTEM", "Endzone2"}
	for _, base := range unityDirs {
		r, ok := byDir["/mnt/linux2/Games/"+base]
		if !ok {
			continue
		}
		if r.TitleSource != "unity" || r.Title == "" {
			t.Errorf("%s: title=%q src=%q, want unity product", base, r.Title, r.TitleSource)
		}
	}
	fuzzyWant := map[string]string{
		"/mnt/linux3/Games/Black Myth Wukong":    "Black Myth: Wukong",
		"/mnt/linux3/Games/Silent Hill 2":        "Silent Hill 2",
		"/mnt/linux2/Games/The Witness":          "The Witness",
		"/mnt/linux2/Games/Nobody Wants to Die":  "Nobody Wants to Die",
		"/mnt/linux2/Games/Assassins Creed Shadows": "Assassin's Creed Shadows",
	}
	for dir, want := range fuzzyWant {
		r, ok := byDir[dir]
		if !ok {
			continue
		}
		if r.Title != want {
			t.Errorf("%s: title=%q (src=%q), want fuzzy-canonical %q", dir, r.Title, r.TitleSource, want)
		}
	}
	for _, r := range rows {
		low := strings.ToLower(r.InstallDir)
		for _, bad := range []string{"proton", "steamlinuxruntime", "compatdata", "downloading", "shadercache", "workshop", "thirdparty", "microsoftnetframework"} {
			if strings.Contains(low, bad) {
				t.Errorf("plumbing row: %+v", r)
			}
		}
	}
	if _, ok := byDir["/mnt/linux3/Games/PREY"]; !ok {
		t.Error("PREY missing")
	}
}
