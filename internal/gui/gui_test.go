package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func entry(name, appid string, status domain.Status, mod time.Time) app.LibraryEntry {
	return app.LibraryEntry{
		Game:    domain.Game{AppID: appid, Name: name, InstallDir: "/games/" + name},
		Status:  status,
		ModTime: mod,
	}
}

func TestActionListSortsActionableFirst(t *testing.T) {
	now := time.Now()
	rows := []app.LibraryEntry{
		entry("Bravo", "2", "", now),                       // normal, newest
		entry("Alpha", "1", domain.StatusFailed, now.Add(-time.Hour)), // failed, oldest
		entry("Charlie", "3", "", now.Add(-time.Minute)),   // normal, older
	}
	sortRows(rows)

	if rows[0].Game.Name != "Alpha" {
		t.Errorf("actionable (failed) install should sort first, got %s", rows[0].Game.Name)
	}
	if rows[1].Game.Name != "Bravo" || rows[2].Game.Name != "Charlie" {
		t.Errorf("recency order broken: %v", []string{rows[1].Game.Name, rows[2].Game.Name})
	}
	for i, r := range rows {
		t.Logf("%d: %s (status=%q)", i, r.Game.Name, r.Status)
	}
}

func TestFilterNarrowsList(t *testing.T) {
	rows := []app.LibraryEntry{
		entry("Cyberpunk 2077", "1091500", "", time.Now()),
		entry("The Witcher 3", "292030", "", time.Now()),
		entry("ELDEN RING", "1245620", "", time.Now()),
	}

	cases := []struct {
		query string
		want  []string
	}{
		{"cyber", []string{"Cyberpunk 2077"}},
		{"WITCHER", []string{"The Witcher 3"}},
		{"elden", []string{"ELDEN RING"}},
		{"1091500", []string{"Cyberpunk 2077"}},
		{"", []string{"Cyberpunk 2077", "The Witcher 3", "ELDEN RING"}},
		{"nothing-matches-this", nil},
	}
	for _, tc := range cases {
		got := filterRows(rows, tc.query)
		var names []string
		for _, r := range got {
			names = append(names, r.Game.Name)
		}
		if len(names) != len(tc.want) {
			t.Errorf("filter %q: got %v, want %v", tc.query, names, tc.want)
			continue
		}
		for i := range names {
			if names[i] != tc.want[i] {
				t.Errorf("filter %q: got %v, want %v", tc.query, names, tc.want)
				break
			}
		}
		t.Logf("filter %q → %v", tc.query, names)
	}
}

func TestEACModalShownBeforeInstall(t *testing.T) {
	protected := entry("Protected Game", "1", "", time.Now())
	protected.EAC = true
	if decideInstall(protected) != confirmEAC {
		t.Error("EAC-protected game must route to the confirmation modal")
	}
	clean := entry("Clean Game", "2", "", time.Now())
	if decideInstall(clean) != installNow {
		t.Error("clean game must install without the modal")
	}
	t.Log("EAC gating: protected → modal, clean → direct")
}

func TestRenderToPNGSmoke(t *testing.T) {
	m := newModel(Config{AuditGrid: false})
	m.rows = []app.LibraryEntry{
		entry("Cyberpunk 2077", "1091500", domain.StatusCommitted, time.Now()),
		entry("Broken Game", "42", domain.StatusFailed, time.Now()),
	}
	sortRows(m.rows)

	out := filepath.Join(t.TempDir(), "frame.png")
	if err := renderToPNG(out, 900, 600, m.rootView); err != nil {
		t.Fatalf("renderToPNG: %v", err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("no PNG produced: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("PNG is empty")
	}
	t.Logf("smoke frame: %s (%d bytes)", out, st.Size())
}
