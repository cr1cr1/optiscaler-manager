package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// containerFixture returns a library root holding two game subdirectories
// (Alpha, Beta), each nesting its exe one level down like T1's
// two-gamey-children fixture — a scan root, not a game itself.
func containerFixture(t *testing.T) (root, alpha, beta string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "Games")
	alpha = filepath.Join(root, "Alpha")
	beta = filepath.Join(root, "Beta")
	writeUIFile(t, filepath.Join(alpha, "bin", "alpha.exe"), "GAME")
	writeUIFile(t, filepath.Join(beta, "bin", "beta.exe"), "GAME")
	return root, alpha, beta
}

func rowDirs(rows []GameRow) map[string]GameRow {
	out := map[string]GameRow{}
	for _, r := range rows {
		out[r.InstallDir] = r
	}
	return out
}

// TestMergeExtraDirs_SkipsContainer: an extra dir that is a container (no
// own exe, several game-bearing children) must not get a self-row — its
// games already surface as individual rows via the recursive scan.
func TestMergeExtraDirs_SkipsContainer(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, alpha, beta := containerFixture(t)
	e.sess.deps.Settings.ExtraDirs = []string{root}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	rows := rowDirs(e.sess.Snapshot().Rows)
	if _, ok := rows[canonicalDir(alpha)]; !ok {
		t.Errorf("child game Alpha missing from rows: %v", rows)
	}
	if _, ok := rows[canonicalDir(beta)]; !ok {
		t.Errorf("child game Beta missing from rows: %v", rows)
	}
	if r, ok := rows[canonicalDir(root)]; ok {
		t.Errorf("container root must not get a self-row, got %+v", r)
	}
}

// TestMergeExtraDirs_GameDirRowKept: a game-like extra dir the recursive
// scan does not surface (its exe sits directly inside it, not in a
// subdirectory) keeps its manual self-row with its title.
func TestMergeExtraDirs_GameDirRowKept(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	game := filepath.Join(t.TempDir(), "SoloGame")
	writeUIFile(t, filepath.Join(game, "solo.exe"), "GAME")
	e.sess.deps.Settings.ExtraDirs = []string{game}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	r, ok := rowDirs(e.sess.Snapshot().Rows)[canonicalDir(game)]
	if !ok {
		t.Fatal("game-like extra dir lost its self-row")
	}
	if r.Title != "SoloGame" {
		t.Errorf("row title = %q, want %q", r.Title, "SoloGame")
	}
}

// TestScan_StaleContainerRowNotResurrected: a row for a container extra dir
// (e.g. written into games.json before container gating existed) must not
// be kept by the scan's in-flight merge — container install dirs are scan
// roots, and their stale rows are dropped.
func TestScan_StaleContainerRowNotResurrected(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, alpha, _ := containerFixture(t)
	e.sess.deps.Settings.ExtraDirs = []string{root}

	// Seed the stale container row as a warm cache from a pre-gating
	// build would have left it.
	e.sess.st.Rows = append(e.sess.st.Rows, GameRow{
		Title:      "Games",
		AppID:      "custom_Games",
		InstallDir: canonicalDir(root),
		Platform:   domain.StoreManual.String(),
		Store:      domain.StoreManual,
	})

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	rows := rowDirs(e.sess.Snapshot().Rows)
	if r, ok := rows[canonicalDir(root)]; ok {
		t.Errorf("stale container row resurrected by scan: %+v", r)
	}
	if _, ok := rows[canonicalDir(alpha)]; !ok {
		t.Errorf("child game Alpha missing from rows: %v", rows)
	}
}

func toastWith(toasts []Toast, substr string) (Toast, bool) {
	for _, t := range toasts {
		if strings.Contains(t.Text, substr) {
			return t, true
		}
	}
	return Toast{}, false
}

// TestAddDirectory_Container_NoSelfRow_ScanFolderToast: adding a container
// registers it as a scan root — ExtraDirs updated and persisted — without a
// placeholder or self-row, and the toast says "scan folder", never "added".
func TestAddDirectory_Container_NoSelfRow_ScanFolderToast(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, _, _ := containerFixture(t)

	e.sess.AddDirectory(root)

	want := canonicalDir(root)
	got := e.sess.Settings().ExtraDirs
	if len(got) != 1 || got[0] != want {
		t.Fatalf("in-memory ExtraDirs = %v, want [%s]", got, want)
	}
	loaded, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(loaded.ExtraDirs) != 1 || loaded.ExtraDirs[0] != want {
		t.Fatalf("persisted ExtraDirs = %v, want [%s]", loaded.ExtraDirs, want)
	}
	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == want {
			t.Fatalf("container add created a self-row: %+v", r)
		}
	}

	waitEvent(t, e.sess, EvScanDone) // the triggered rescan settles
	toasts := e.sess.Snapshot().Toasts
	if _, ok := toastWith(toasts, "as a scan folder"); !ok {
		t.Errorf("missing scan-folder toast: %+v", toasts)
	}
	if _, ok := toastWith(toasts, "added "); ok {
		t.Errorf("container add must not toast 'added X': %+v", toasts)
	}
	// No self-row even after the triggered scan settles.
	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == want {
			t.Fatalf("container self-row after rescan: %+v", r)
		}
	}
}

// TestAddDirectory_Container_ChildrenSurfacedAfterRescan: the rescan a
// container add triggers surfaces the root's games as individual rows.
func TestAddDirectory_Container_ChildrenSurfacedAfterRescan(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, alpha, beta := containerFixture(t)

	e.sess.AddDirectory(root)
	waitEvent(t, e.sess, EvScanDone)

	rows := rowDirs(e.sess.Snapshot().Rows)
	if _, ok := rows[canonicalDir(alpha)]; !ok {
		t.Errorf("child game Alpha not surfaced: %v", rows)
	}
	if _, ok := rows[canonicalDir(beta)]; !ok {
		t.Errorf("child game Beta not surfaced: %v", rows)
	}
	if r, ok := rows[canonicalDir(root)]; ok {
		t.Errorf("container row present after rescan: %+v", r)
	}
}

// TestAddDirectory_NoGamesAnywhere_Refused_NotPersisted: a directory with no
// game exe anywhere is refused — no ExtraDirs change (memory or disk), no
// row, no op slot held — with a warning toast.
func TestAddDirectory_NoGamesAnywhere_Refused_NotPersisted(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	empty := filepath.Join(t.TempDir(), "NothingHere")
	writeUIFile(t, filepath.Join(empty, "notes.txt"), "hello")

	e.sess.AddDirectory(empty)

	if got := e.sess.Settings().ExtraDirs; len(got) != 0 {
		t.Fatalf("ExtraDirs after refusal = %v, want empty", got)
	}
	loaded, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(loaded.ExtraDirs) != 0 {
		t.Fatalf("persisted ExtraDirs after refusal = %v, want empty", loaded.ExtraDirs)
	}
	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == canonicalDir(empty) {
			t.Fatalf("refused directory got a row: %+v", r)
		}
	}
	if e.sess.OpBusy(canonicalDir(empty)) {
		t.Error("refused directory still holds an op slot")
	}
	toast, ok := toastWith(e.sess.Snapshot().Toasts, "no games found under NothingHere")
	if !ok {
		t.Fatalf("missing refusal toast: %+v", e.sess.Snapshot().Toasts)
	}
	if !toast.Warn {
		t.Errorf("refusal toast Warn = false, want true: %+v", toast)
	}
}

// TestAddDirectory_GameDir_RowAppears: pinning the game-dir path — a real
// game directory still gets its row (placeholder then enrichment) and
// settles with the usual "directory added" event.
func TestAddDirectory_GameDir_RowAppears(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	game := filepath.Join(t.TempDir(), "PinnedGame")
	writeUIFile(t, filepath.Join(game, "pinned.exe"), "GAME")

	e.sess.AddDirectory(game)
	waitEventText(t, e.sess, EvScanDone, "directory added")

	r, ok := rowDirs(e.sess.Snapshot().Rows)[canonicalDir(game)]
	if !ok {
		t.Fatal("game dir row missing after add")
	}
	if r.Title != "PinnedGame" {
		t.Errorf("row title = %q, want folder fallback %q", r.Title, "PinnedGame")
	}
	loaded, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(loaded.ExtraDirs) != 1 || loaded.ExtraDirs[0] != canonicalDir(game) {
		t.Fatalf("persisted ExtraDirs = %v", loaded.ExtraDirs)
	}
}

// TestAddDirectory_PlaceholderReplacedByPETitle: the synchronous placeholder
// row carries the folder title; once enrichment settles ("directory added"
// event) the same row carries the main executable's PE ProductName.
func TestAddDirectory_PlaceholderReplacedByPETitle(t *testing.T) {
	e := newSlowCoversEnv(t, time.Second)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "ShinyFolder")
	pe := testutil.StringInfoPE(true, map[string]string{"ProductName": "Shiny PE Title"}, [4]uint16{})
	writeUIFile(t, filepath.Join(custom, "shiny.exe"), string(pe))

	e.sess.AddDirectory(custom)

	r, ok := rowDirs(e.sess.Snapshot().Rows)[canonicalDir(custom)]
	if !ok {
		t.Fatal("placeholder row missing right after AddDirectory")
	}
	if r.Title != "ShinyFolder" {
		t.Errorf("placeholder title = %q, want folder title %q", r.Title, "ShinyFolder")
	}

	waitEventText(t, e.sess, EvScanDone, "directory added")
	r, ok = rowDirs(e.sess.Snapshot().Rows)[canonicalDir(custom)]
	if !ok {
		t.Fatal("row missing after enrichment settled")
	}
	if r.Title != "Shiny PE Title" {
		t.Errorf("enriched title = %q, want PE ProductName %q", r.Title, "Shiny PE Title")
	}
}

// TestMergeExtraDirs_TicksEveryNonScanOnlyRoot: coversTotal counts every
// non-scanOnly extra root, so the tick must fire for each of them — a
// deduplicated self-row (the common case since v0.7.1, where the scan
// surfaces game-dir roots itself) and a failing ManualEntry alike —
// otherwise the covers phase stalls below 100%.
func TestMergeExtraDirs_TicksEveryNonScanOnlyRoot(t *testing.T) {
	e := newTestEnv(t)
	game := filepath.Join(t.TempDir(), "SoloGame")
	writeUIFile(t, filepath.Join(game, "solo.exe"), "GAME")
	entry, err := app.ManualEntry(game, e.sess.deps.Store)
	if err != nil {
		t.Fatal(err)
	}
	rows := []GameRow{{InstallDir: entry.Game.InstallDir}} // self-row already surfaced: dedup path
	container, _, _ := containerFixture(t)

	ticks := 0
	out := e.sess.mergeExtraDirs(context.Background(), rows,
		[]string{game, filepath.Join(t.TempDir(), "missing"), container},
		map[string]bool{container: true},
		func() { ticks++ },
		discovery.ChainResolver(nil))

	if ticks != 2 {
		t.Errorf("ticks = %d, want 2 (deduplicated root + failing root; scanOnly root skipped)", ticks)
	}
	if len(out) != 1 {
		t.Errorf("rows = %d, want 1 (dedup added nothing)", len(out))
	}
}

// TestResolveGameExe_EngineNamedParent: a manually added game whose parent
// dir is engine-named (e.g. /games/bin/MyGame) makes the parent scan yield
// nothing (engine-named roots are refused); the launch fallback must still
// find the exe inside the game dir itself.
func TestResolveGameExe_EngineNamedParent(t *testing.T) {
	game := filepath.Join(t.TempDir(), "bin", "MyGame")
	writeUIFile(t, filepath.Join(game, "solo.exe"), "GAME")

	exe, err := resolveGameExe(game)
	if err != nil {
		t.Fatalf("resolveGameExe under engine-named parent: %v", err)
	}
	if filepath.Base(exe) != "solo.exe" {
		t.Errorf("exe = %q, want solo.exe", exe)
	}
}

// TestScan_DuplicateTitlesDisambiguated: two games whose exes share one PE
// ProductName must not be indistinguishable in the library — each row gets
// its folder name as a suffix.
func TestScan_DuplicateTitlesDisambiguated(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root := t.TempDir()
	pe := testutil.StringInfoPE(false, map[string]string{"ProductName": "TOI"}, [4]uint16{1, 0, 0, 0})
	writeUIFile(t, filepath.Join(root, "Tails of Iron", "game.exe"), string(pe))
	writeUIFile(t, filepath.Join(root, "Tails of Iron Bright Fir Forest", "game.exe"), string(pe))
	e.sess.deps.Settings.ExtraDirs = []string{root}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	titles := map[string]bool{}
	for _, r := range e.sess.Snapshot().Rows {
		if strings.HasPrefix(r.Title, "TOI") {
			titles[r.Title] = true
		}
	}
	if !titles["TOI (Tails of Iron)"] || !titles["TOI (Tails of Iron Bright Fir Forest)"] {
		t.Errorf("duplicate TOI rows not disambiguated: %v", titles)
	}
}

// TestScan_TitleOverrideWins: a pinned title in settings beats every
// identification rule for the row, and the row records the source.
func TestScan_TitleOverrideWins(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	game := filepath.Join(t.TempDir(), "SomeGame")
	writeUIFile(t, filepath.Join(game, "game.exe"), string(testutil.StringInfoPE(false, map[string]string{"ProductName": "PE Title"}, [4]uint16{1, 0, 0, 0})))
	canon := canonicalDir(game)
	e.sess.deps.Settings.ExtraDirs = []string{game}
	e.sess.deps.Settings.TitleOverrides = map[string]string{canon: "Pinned Title"}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	r, ok := rowDirs(e.sess.Snapshot().Rows)[canon]
	if !ok {
		t.Fatal("game row missing")
	}
	if r.Title != "Pinned Title" || r.TitleSource != "override" {
		t.Errorf("row = %+v, want pinned title with override source", r)
	}
}
