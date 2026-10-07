package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// dlssEnv extends the standard session fake with a DLSS client pointed at a
// fake NVIDIA endpoint and the three NVIDIA DLLs planted in the game's
// injection directory.
type dlssEnv struct {
	*testEnv
	dlssRoot string
	raws     atomic.Int64 // runtime-byte hits (the startup preload fetches the latest set once, on miss)
	refuse   atomic.Bool  // set to make every NVIDIA endpoint return 500
}

func newDLSSEnv(t *testing.T, withDLLs bool) *dlssEnv {
	t.Helper()
	e := &dlssEnv{testEnv: newTestEnv(t), dlssRoot: t.TempDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/NVIDIA/DLSS/commits/main", func(w http.ResponseWriter, r *http.Request) {
		if e.refuse.Load() {
			http.Error(w, "refused", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"sha":"` + strings.Repeat("d", 40) + `"}`))
	})
	mux.HandleFunc("/repos/NVIDIA/DLSS/tags", func(w http.ResponseWriter, r *http.Request) {
		if e.refuse.Load() {
			http.Error(w, "refused", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"v310.9.1","commit":{"sha":"` + strings.Repeat("e", 40) + `"}}]`))
	})
	mux.HandleFunc("/NVIDIA/DLSS/", func(w http.ResponseWriter, r *http.Request) {
		e.raws.Add(1)
		if e.refuse.Load() {
			http.Error(w, "refused", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 5, 3, 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	e.sess.deps.DLSS = dlss.NewWithBaseURLs(srv.Client(), srv.URL, srv.URL)
	e.sess.deps.SettingsRoot = e.dlssRoot
	if withDLLs {
		for i, name := range dlss.Files {
			writeUIFile(t, filepath.Join(e.bin, name), string(testutil.FixedVersionPE(3, 7, uint16(20+i), 0)))
		}
	}
	return e
}

func scanOneDLSSRow(t *testing.T, e *dlssEnv) GameRow {
	t.Helper()
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	rows := e.sess.Snapshot().Rows
	if len(rows) != 1 {
		t.Fatalf("rows %d, want 1", len(rows))
	}
	return rows[0]
}

func dlssDLLVersion(t *testing.T, e *dlssEnv, name string) string {
	t.Helper()
	v, err := pever.FileVersion(filepath.Join(e.bin, name))
	if err != nil {
		t.Fatalf("%s unreadable after op: %v", name, err)
	}
	return v
}

// TestUpdateDLSSAndRestoreRoundTrip: the update replaces all three NVIDIA
// DLLs from one immutable commit after backing them up, the row's DLSS pill
// version refreshes, and a confirmed restore brings the complete prior set
// back.
func TestUpdateDLSSAndRestoreRoundTrip(t *testing.T) {
	e := newDLSSEnv(t, true)
	row := scanOneDLSSRow(t, e)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	for _, name := range dlss.Files {
		if got := dlssDLLVersion(t, e, name); got != "310.5.3.0" {
			t.Errorf("%s after update = %q, want 310.5.3.0", name, got)
		}
	}
	updated := e.sess.Snapshot().Rows[0]
	if len(updated.Components) == 0 || updated.Components[0] != "DLSS 310.5.3" {
		t.Errorf("components after update %v, want [DLSS 310.5.3] (raw label)", updated.Components)
	}

	snaps := e.sess.DLSSSnapshots(row.InstallDir)
	if len(snaps) != 1 {
		t.Fatalf("snapshots after update %d, want 1 (the pre-update set)", len(snaps))
	}
	e.sess.RestoreDLSS(row.InstallDir, snaps[0].ID)
	waitEvent(t, e.sess, EvConfirm)
	confirm := e.sess.Snapshot().Confirm
	if confirm == nil {
		t.Fatal("restore produced no confirmation")
	}
	if confirm.Kind != ConfirmDLSSRestore || confirm.SnapshotID != snaps[0].ID {
		t.Errorf("confirm %+v, want ConfirmDLSSRestore for %q", confirm, snaps[0].ID)
	}
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	for i, name := range dlss.Files {
		want := fmt.Sprintf("3.7.%d.0", 20+i)
		if got := dlssDLLVersion(t, e, name); got != want {
			t.Errorf("%s after restore = %q, want %q (the backed-up original)", name, got, want)
		}
	}
	restored := e.sess.Snapshot().Rows[0]
	if len(restored.Components) == 0 || restored.Components[0] != "DLSS 3.7.20" {
		t.Errorf("components after restore %v, want [DLSS 3.7.20]", restored.Components)
	}
	// The restore itself backed up the updated set: a second menu entry.
	if got := len(e.sess.DLSSSnapshots(row.InstallDir)); got != 2 {
		t.Errorf("snapshots after restore %d, want 2", got)
	}
	t.Log("update backed up originals, restore swapped the complete set back")
}

// TestUpdateDLSSMissingDLLProceeds: an incomplete NVIDIA set does not block
// the update — it warns (log), installs the missing members with the rest,
// and writes no rollback backup of the incomplete set.
func TestUpdateDLSSMissingDLLProceeds(t *testing.T) {
	e := newDLSSEnv(t, false)
	row := scanOneDLSSRow(t, e)
	if err := os.WriteFile(filepath.Join(e.bin, dlss.Files[0]), testutil.FixedVersionPE(3, 7, 20, 0), 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.UpdateDLSS(row.InstallDir)
	ev := waitEvent(t, e.sess, EvOpDone)
	if ev.GameDir != row.InstallDir {
		t.Errorf("done event GameDir %q, want %q (frontends key refreshes on it)", ev.GameDir, row.InstallDir)
	}
	for _, name := range dlss.Files {
		if got := dlssDLLVersion(t, e, name); got != "310.5.3.0" {
			t.Errorf("%s after update = %q, want 310.5.3.0", name, got)
		}
	}
	if snaps := e.sess.DLSSSnapshots(row.InstallDir); len(snaps) != 0 {
		t.Errorf("backup-less update created %d snapshots, want 0", len(snaps))
	}
}

// TestRestoreDLSSDeclineLeavesUntouched: declining the confirmation must
// run no operation and change no bytes.
func TestRestoreDLSSDeclineLeavesUntouched(t *testing.T) {
	e := newDLSSEnv(t, true)
	row := scanOneDLSSRow(t, e)
	e.sess.RestoreDLSS(row.InstallDir, "nonexistent")
	if c := e.sess.Snapshot().Confirm; c != nil {
		t.Fatalf("unknown snapshot id opened a confirm: %+v", c)
	}
	// A real snapshot seeds the confirm; declining must be a no-op.
	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	snaps := e.sess.DLSSSnapshots(row.InstallDir)
	e.sess.RestoreDLSS(row.InstallDir, snaps[0].ID)
	if e.sess.Snapshot().Confirm == nil {
		t.Fatal("restore of a real snapshot produced no confirmation")
	}
	e.sess.AnswerConfirm(false)
	if c := e.sess.Snapshot().Confirm; c != nil {
		t.Fatalf("confirm not cleared on decline: %+v", c)
	}
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "310.5.3.0" {
		t.Errorf("%s changed on declined restore: %q", dlss.Files[0], got)
	}
	if got := len(e.sess.DLSSSnapshots(row.InstallDir)); got != 1 {
		t.Errorf("declined restore created %d snapshots, want 1", got)
	}
}

// seedDLSSCache plants a complete commit-keyed download-cache dir (three
// runtime files plus the manifest pinning their real digests), the shape
// ensureCache leaves behind.
func seedDLSSCache(t *testing.T, cacheRoot, commit string, maj, min uint16) {
	t.Helper()
	testutil.SeedDLSSCacheDir(t, cacheRoot, commit, dlssCacheFiles(maj, min))
}

// dlssCacheFiles builds the three-file NVIDIA runtime set at one version.
func dlssCacheFiles(maj, min uint16) map[string][]byte {
	files := map[string][]byte{}
	for i, name := range dlss.Files {
		files[name] = testutil.FixedVersionPE(maj, min, uint16(i), 0)
	}
	return files
}

// TestCheckDLSSStartupPreloadsLatestSet (user spec): the startup check
// fills the published version and commit from one small tags call, and —
// when the latest published set is not already cached — PRE-DOWNLOADS it
// into the download cache (three runtime files, no more) so the DLSS pill
// serves the latest offline-ready. The cached half still reports the
// NEWEST complete set in the local cache.
func TestCheckDLSSStartupPreloadsLatestSet(t *testing.T) {
	e := newDLSSEnv(t, true)
	e.sess.SetOnlineLookups(true) // the fixture defaults it off; the check is an online lookup
	seedDLSSCache(t, e.sess.deps.CacheDir, strings.Repeat("a", 40), 310, 6)

	e.sess.CheckDLSS(context.Background())
	waitEvent(t, e.sess, EvDLSSStatus)
	st := e.sess.Snapshot()
	if st.DLSSLatest.Version != "310.9.1" || st.DLSSLatest.Commit != strings.Repeat("e", 40) {
		t.Fatalf("DLSSLatest = %+v, want 310.9.1 @ e-padded commit", st.DLSSLatest)
	}
	if st.DLSSCached != "310.6.0.0" {
		t.Errorf("DLSSCached = %q, want 310.6.0.0 (newest complete cache set)", st.DLSSCached)
	}
	if st.DLSSCachedCommit != strings.Repeat("a", 40) {
		t.Errorf("DLSSCachedCommit = %q, want the a-padded commit", st.DLSSCachedCommit)
	}
	// The startup preload fetched exactly the three runtime files of the
	// latest published set into its commit dir. The preload runs AFTER the
	// DLSSStatus poke (same goroutine), so wait for it to settle.
	latestDir := filepath.Join(e.sess.deps.CacheDir, "dlss", strings.Repeat("e", 40))
	waitSettled := func() bool {
		for _, name := range dlss.Files {
			if _, err := os.Stat(filepath.Join(latestDir, name)); err != nil {
				return false
			}
		}
		return true
	}
	deadline := time.Now().Add(10 * time.Second)
	for !waitSettled() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !waitSettled() {
		t.Fatalf("startup preload did not cache the latest set in %q (raws=%d)", latestDir, e.raws.Load())
	}
	if raws := e.raws.Load(); raws != 3 {
		t.Errorf("startup preload fetched %d runtime files, want 3 (the complete latest set)", raws)
	}
}

// TestCheckDLSSStartupPreloadSkippedWhenCached: with the latest published
// set already complete in the download cache the startup preload fetches
// ZERO runtime bytes (the pre-download is a fill-missing, not a refetch).
func TestCheckDLSSStartupPreloadSkippedWhenCached(t *testing.T) {
	e := newDLSSEnv(t, true)
	e.sess.SetOnlineLookups(true)
	seedDLSSCache(t, e.sess.deps.CacheDir, strings.Repeat("e", 40), 310, 9) // the latest commit, already cached

	e.sess.CheckDLSS(context.Background())
	waitEvent(t, e.sess, EvDLSSStatus)
	if st := e.sess.Snapshot(); st.DLSSLatest.Commit != strings.Repeat("e", 40) {
		t.Fatalf("DLSSLatest = %+v, want the e-padded commit", st.DLSSLatest)
	}
	if raws := e.raws.Load(); raws != 0 {
		t.Errorf("startup preload re-fetched %d runtime files for an already-cached latest set", raws)
	}
}

// TestCheckDLSSOfflineLookupsOff: with online lookups disabled the check
// stays local — no tags call, no published version — but the cached half
// still fills, so an offline press can serve from the download cache.
func TestCheckDLSSOfflineLookupsOff(t *testing.T) {
	e := newDLSSEnv(t, true)
	e.sess.SetOnlineLookups(false)
	seedDLSSCache(t, e.sess.deps.CacheDir, strings.Repeat("a", 40), 310, 6)

	e.sess.CheckDLSS(context.Background())
	if st := e.sess.Snapshot(); st.DLSSLatest.Version != "" || st.DLSSLatest.Commit != "" {
		t.Fatalf("offline check filled DLSSLatest: %+v", st.DLSSLatest)
	}
	if st := e.sess.Snapshot(); st.DLSSCached != "310.6.0.0" || st.DLSSCachedCommit != strings.Repeat("a", 40) {
		t.Fatalf("offline check left the cached half empty: %+v / %q", st.DLSSCached, st.DLSSCachedCommit)
	}
}

// TestUpdateDLSSRefreshesCachedStatus: a successful update re-fills the
// download cache, so the session's cached-version status moves to the
// installed version.
func TestUpdateDLSSRefreshesCachedStatus(t *testing.T) {
	e := newDLSSEnv(t, true)
	row := scanOneDLSSRow(t, e)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	if got := e.sess.Snapshot().DLSSCached; got != "310.5.3.0" {
		t.Errorf("DLSSCached after update = %q, want 310.5.3.0", got)
	}
}

// TestUpdateDLSSUsesStartupCommitHint: with the published commit already in
// the download cache, pressing the DLSS badge installs from the cache with
// zero network — even with every endpoint refusing.
func TestUpdateDLSSUsesStartupCommitHint(t *testing.T) {
	e := newDLSSEnv(t, true)
	row := scanOneDLSSRow(t, e)
	hint := strings.Repeat("e", 40)
	seedDLSSCache(t, e.sess.deps.CacheDir, hint, 310, 9)
	e.sess.mu.Lock()
	e.sess.st.DLSSLatest = dlss.Latest{Version: "310.9.1", Commit: hint}
	e.sess.mu.Unlock()
	// Every endpoint refuses: the cache-hit update must not touch them.
	e.refuse.Store(true)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "310.9.0.0" {
		t.Errorf("%s after cache-hit update = %q, want 310.9.0.0", dlss.Files[0], got)
	}
}

// TestUpdateDLSSUsesCachedCommitOffline: with no published version known
// (online lookups off, failed startup check) a press still serves from the
// download cache when the startup check recorded a cached commit — the
// update itself needs no network for a cached set.
func TestUpdateDLSSUsesCachedCommitOffline(t *testing.T) {
	e := newDLSSEnv(t, true)
	e.sess.SetOnlineLookups(false)
	row := scanOneDLSSRow(t, e)
	commit := strings.Repeat("a", 40)
	seedDLSSCache(t, e.sess.deps.CacheDir, commit, 310, 9)
	e.sess.CheckDLSS(context.Background()) // local half only: fills DLSSCachedCommit
	if got := e.sess.Snapshot().DLSSCachedCommit; got != commit {
		t.Fatalf("setup: cached commit %q, want %q", got, commit)
	}
	// Every endpoint refuses: the cache-served update must not touch them.
	e.refuse.Store(true)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "310.9.0.0" {
		t.Errorf("%s after offline cache update = %q, want 310.9.0.0", dlss.Files[0], got)
	}
}

// TestUpdateDLSSAlreadyLatest: a press on an already-current set settles
// gracefully — an informational done event naming the version, no failure,
// no duplicate backup, no file changes.
func TestUpdateDLSSAlreadyLatest(t *testing.T) {
	e := newDLSSEnv(t, true)
	row := scanOneDLSSRow(t, e)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	if snaps := e.sess.DLSSSnapshots(row.InstallDir); len(snaps) != 1 {
		t.Fatalf("snapshots after first update %d, want 1", len(snaps))
	}

	e.sess.UpdateDLSS(row.InstallDir)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "already at 310.5.3.0") {
		t.Errorf("already-latest text %q, want it to name the version", ev.Text)
	}
	if snaps := e.sess.DLSSSnapshots(row.InstallDir); len(snaps) != 1 {
		t.Errorf("snapshots after no-op press %d, want 1", len(snaps))
	}
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "310.5.3.0" {
		t.Errorf("%s changed on no-op press: %q", dlss.Files[0], got)
	}
}
