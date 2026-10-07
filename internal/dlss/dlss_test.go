package dlss

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/jsoncache"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// fileVersion keeps the test assertions readable; the prod-side delegate
// was removed (pever.FileVersion is called directly).
func fileVersion(path string) (string, error) { return pever.FileVersion(path) }

func TestUpdateBacksUpAndReplacesAllNVIDIADLLs(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)

	sha := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/NVIDIA/DLSS/commits/main":
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
		case strings.Contains(r.URL.Path, "/"+sha+"/lib/Windows_x86_64/rel/"):
			_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceCommit != sha {
		t.Fatalf("source commit %q, want %q", snap.SourceCommit, sha)
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s version %q, err=%v", name, v, err)
		}
		v, err = fileVersion(filepath.Join(root, "dlss-backups", snapshotGameID(game), snap.ID, name))
		if err != nil || !strings.HasPrefix(v, "1.0.") {
			t.Fatalf("backup %s version %q, err=%v", name, v, err)
		}
	}
}

// TestUpdateReusesCachedCommitWithoutSecondDownload: like the OptiScaler
// bundle cache, the NVIDIA runtime files are downloaded once per commit and
// reused for every later update of that commit — a cache hit must not touch
// the network again.
func TestUpdateReusesCachedCommitWithoutSecondDownload(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)

	sha := strings.Repeat("e", 40)
	raws := 0
	refuse := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/NVIDIA/DLSS/commits/main":
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
		case refuse:
			raws++
			http.Error(w, "cache reuse must not download again", http.StatusInternalServerError)
		default:
			raws++
			_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
		}
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	first, err := Update(context.Background(), client, cache, root, game, "")
	if err != nil {
		t.Fatal(err)
	}
	if raws != len(Files) {
		t.Fatalf("first update fetched %d files, want %d", raws, len(Files))
	}
	for _, name := range Files {
		if _, err := os.Stat(filepath.Join(cache, "dlss", first.SourceCommit, name)); err != nil {
			t.Fatalf("cached %s unreadable after update: %v", name, err)
		}
	}

	// Second update of the same commit: the network is closed for business,
	// and the installed set already matches the target — a graceful no-op
	// (no refetch, no reinstall, no duplicate backup).
	refuse = true
	_, err = Update(context.Background(), client, cache, root, game, "")
	var already *AlreadyLatestError
	if !errors.As(err, &already) || already.Version != "310.9.1.0" {
		t.Fatalf("cached update err = %v, want AlreadyLatestError{310.9.1.0}", err)
	}
	if raws != len(Files) {
		t.Fatalf("cache hit fetched %d extra files, want 0", raws-len(Files))
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s version %q after cached update (err=%v)", name, v, err)
		}
	}
}

// TestUpdateFetchesNewCommitWhenMainMoves pins the moved-commit half of
// the cache contract: main re-resolves on every update, so a moved commit
// maps to a fresh cache dir whose bytes are fetched — stale cached bytes
// are never served.
func TestUpdateFetchesNewCommitWhenMainMoves(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)

	shaA, shaB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	moved := false
	raws := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			sha := shaA
			if moved {
				sha = shaB
			}
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		raws++
		if strings.Contains(r.URL.Path, "/"+shaB+"/") {
			_, _ = w.Write(testutil.FixedVersionPE(310, 10, 0, 0))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	if _, err := Update(context.Background(), client, root, root, game, ""); err != nil {
		t.Fatal(err)
	}
	if raws != len(Files) {
		t.Fatalf("first update fetched %d files, want %d", raws, len(Files))
	}
	moved = true
	if _, err := Update(context.Background(), client, root, root, game, ""); err != nil {
		t.Fatal(err)
	}
	if raws != 2*len(Files) {
		t.Fatalf("moved commit fetched %d extra files, want %d", raws-len(Files), len(Files))
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.10.0.0" {
			t.Fatalf("%s version %q after move, want 310.10.0.0 (err=%v)", name, v, err)
		}
	}
	for _, sha := range []string{shaA, shaB} {
		if _, err := os.Stat(filepath.Join(root, "dlss", sha, "manifest.json")); err != nil {
			t.Fatalf("cache for %s incomplete: %v", sha[:8], err)
		}
	}
}

// TestUpdateRefetchesTamperedCacheEntry: a cache file whose bytes no longer
// match the recorded download hash is refetched, never installed. The
// tampered bytes are a valid PE so only the hash gate can catch them.
func TestUpdateRefetchesTamperedCacheEntry(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)
	sha := strings.Repeat("f", 40)
	raws := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		raws++
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	if _, err := Update(context.Background(), client, cache, root, game, ""); err != nil {
		t.Fatal(err)
	}
	if raws != len(Files) {
		t.Fatalf("first update fetched %d files, want %d", raws, len(Files))
	}
	tampered := filepath.Join(cache, "dlss", sha, Files[0])
	if err := os.WriteFile(tampered, testutil.FixedVersionPE(9, 9, 9, 9), 0o644); err != nil {
		t.Fatal(err)
	}
	// The hash gate refetches the tampered member (one extra download), and
	// the installed set then already matches the repaired cache: the press
	// settles as a graceful no-op with the game bytes untouched.
	_, err := Update(context.Background(), client, cache, root, game, "")
	var already *AlreadyLatestError
	if !errors.As(err, &already) || already.Version != "310.9.1.0" {
		t.Fatalf("update with tampered cache entry: err = %v, want AlreadyLatestError{310.9.1.0}", err)
	}
	if raws != len(Files)+1 {
		t.Fatalf("tampered member fetched %d extra files, want 1", raws-len(Files))
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s installed %q after cache tamper, want 310.9.1.0 (err=%v)", name, v, err)
		}
	}
	v, err := fileVersion(tampered)
	if err != nil || v != "310.9.1.0" {
		t.Fatalf("cache healed to %q, want 310.9.1.0 (err=%v)", v, err)
	}
}

// TestUpdateRefetchesAfterRefusedDownload: a download that fails the PE
// gate must never be recorded in the cache manifest — otherwise a
// consistently lying source would legitimize garbage forever (the hash
// gate passes, the PE gate fails, and no refetch ever triggers). After
// the source heals, the next update must fetch the real bytes.
func TestUpdateRefetchesAfterRefusedDownload(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)

	sha := strings.Repeat("d", 40)
	lies := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		if lies && strings.HasSuffix(r.URL.Path, "/"+Files[2]) {
			_, _ = w.Write([]byte("not a PE image"))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	if _, err := Update(context.Background(), client, root, root, game, ""); err == nil {
		t.Fatal("lying download must fail the update")
	}
	assertGameUnchanged(t, game, "1.0.0.0")
	lies = false
	if _, err := Update(context.Background(), client, root, root, game, ""); err != nil {
		t.Fatalf("update after the source healed failed: %v", err)
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s version %q after healed refetch (err=%v)", name, v, err)
		}
	}
}

func TestRestoreRestoresCompletePriorSnapshot(t *testing.T) {
	root, game := t.TempDir(), ""
	game = filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)
	sha := strings.Repeat("b", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	first, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), root, game, first.ID); err != nil {
		t.Fatal(err)
	}
	for i, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		want := "1.0." + string(rune('0'+i))
		if err != nil || v != want+".0" {
			t.Fatalf("restored %s version %q, want %q (err=%v)", name, v, want+".0", err)
		}
	}
}

// TestUpdateInstallsWhenCurrentDLLsMissing: a partial (or absent) current
// set must not block the update — the missing members are simply installed
// with the rest. The incomplete set has no complete-set rollback value
// (snapshots are all-or-nothing), so no backup is written.
func TestUpdateInstallsWhenCurrentDLLsMissing(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, Files[0]), testutil.FixedVersionPE(1, 0, 0, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("d", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game, "")
	if err != nil {
		t.Fatalf("Update with an incomplete current set: %v", err)
	}
	if snap.ID != "" {
		t.Errorf("snapshot %q written for an incomplete current set; want no backup", snap.ID)
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s version %q after update, want 310.9.1.0 (err=%v)", name, v, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "dlss-backups")); !os.IsNotExist(err) {
		t.Errorf("backup tree created for an incomplete current set (stat err %v)", err)
	}
}

// TestRestoreInstallsWhenCurrentDLLsMissing: restoring a complete snapshot
// over a partial current set proceeds — the missing members come back with
// the restore — and no backup of the incomplete set is written.
func TestRestoreInstallsWhenCurrentDLLsMissing(t *testing.T) {
	root, game, snap := updatedGame(t)
	if err := os.Remove(filepath.Join(game, Files[1])); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), root, game, snap.ID); err != nil {
		t.Fatalf("Restore over an incomplete current set: %v", err)
	}
	for i, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		want := "1.0." + string(rune('0'+i)) + ".0"
		if err != nil || v != want {
			t.Fatalf("restored %s version %q, want %q (err=%v)", name, v, want, err)
		}
	}
	snaps, err := Snapshots(root, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Errorf("%d snapshots after backup-less restore, want the original 1", len(snaps))
	}
}

// TestRestoreRefusesTamperedBackup: a backup whose bytes no longer match
// the snapshot record must never reach the game directory.
func TestRestoreRefusesTamperedBackup(t *testing.T) {
	root, game := t.TempDir(), ""
	game = filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)
	sha := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	first, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game, "")
	if err != nil {
		t.Fatal(err)
	}
	// Tamper with one backed-up member after a successful update.
	backupFile := filepath.Join(root, "dlss-backups", snapshotGameID(game), first.ID, Files[2])
	if err := os.WriteFile(backupFile, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), root, game, first.ID); err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("Restore err %v, want backup verification failure", err)
	}
	for _, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Fatalf("%s changed to %q after refused restore (err=%v); game dir must stay untouched", name, v, err)
		}
	}
}

// TestUpdateCancelledLeavesFilesUntouched: a dead context must produce a
// cancellation error and zero file changes.
func TestUpdateCancelledLeavesFilesUntouched(t *testing.T) {
	root, game := t.TempDir(), ""
	game = filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 2, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Update(ctx, New(nil), root, root, game, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
	for i, name := range Files {
		v, err := fileVersion(filepath.Join(game, name))
		if err != nil || v != "2.0."+string(rune('0'+i))+".0" {
			t.Fatalf("%s changed to %q after cancelled update (err=%v)", name, v, err)
		}
	}
}

// TestRestoreRefusesTamperedSnapshot: a snapshot file corrupted after the
// backup must never reach the game directory.
func TestRestoreRefusesTamperedSnapshot(t *testing.T) {
	root, game, snap := updatedGame(t)
	bak := filepath.Join(root, "dlss-backups", snapshotGameID(game), snap.ID, "nvngx_dlss.dll")
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bak, append(data, 0xFF), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), root, game, snap.ID); err == nil {
		t.Fatal("restore accepted a tampered snapshot")
	}
	assertGameUnchanged(t, game, "310.9.1.0")
}

// TestRestoreRefusesBlankedHashes: a snapshot record whose hashes were
// stripped must not bypass verification.
func TestRestoreRefusesBlankedHashes(t *testing.T) {
	root, game, snap := updatedGame(t)
	dir := filepath.Join(root, "dlss-backups", snapshotGameID(game), snap.ID)
	// Blank only the record; the backup files on disk stay untouched, so
	// the game dir must remain 310.9.1.0 after the refusal.
	snap.Files = nil
	for _, name := range Files {
		snap.Files = append(snap.Files, File{Name: name})
	}
	record, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), record, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), root, game, snap.ID); err == nil {
		t.Fatal("restore accepted a snapshot with blanked hashes")
	}
	assertGameUnchanged(t, game, "310.9.1.0")
}

// updatedGame runs one update against a fake NVIDIA endpoint and returns
// the roots plus the resulting snapshot.
func updatedGame(t *testing.T) (root, game string, snap Snapshot) {
	t.Helper()
	root = t.TempDir()
	game = filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)
	sha := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game, "")
	if err != nil {
		t.Fatal(err)
	}
	return root, game, snap
}

// seedGame writes a complete pre-update DLSS set into dir; the per-file
// patch version distinguishes the members in assertions.
func seedGame(t *testing.T, dir string, maj, min uint16) {
	t.Helper()
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(dir, name), testutil.FixedVersionPE(maj, min, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertGameUnchanged(t *testing.T, game, want string) {
	t.Helper()
	v, err := fileVersion(filepath.Join(game, "nvngx_dlss.dll"))
	if err != nil || v != want {
		t.Fatalf("game DLL changed to %q (err %v), want %q", v, err, want)
	}
}

// TestLatestResolvesPublishedVersionWithoutDownloading: the startup
// availability check reads the published version from the tags API — one
// small metadata call. It must never fetch runtime bytes: the raw-file
// endpoint refuses with 500 and the test counts every hit it would log.
func TestLatestResolvesPublishedVersionWithoutDownloading(t *testing.T) {
	sha := strings.Repeat("7", 40)
	raws := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/NVIDIA/DLSS/tags":
			// Newest-first is not a documented GitHub contract: the check
			// must pick the greatest VERSION among the fetched tags, not
			// trust the listing order.
			_, _ = w.Write([]byte(`[
				{"name":"v310.2.0","commit":{"sha":"` + strings.Repeat("1", 40) + `"}},
				{"name":"v310.9.1","commit":{"sha":"` + sha + `"}},
				{"name":"v310.7.0","commit":{"sha":"` + strings.Repeat("2", 40) + `"}}
			]`))
		default:
			raws++
			http.Error(w, "startup check must not download runtime bytes", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	lat, err := NewWithBaseURLs(server.Client(), server.URL, server.URL).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lat.Version != "310.9.1" || lat.Commit != sha {
		t.Fatalf("latest = %+v, want version 310.9.1 commit %s", lat, sha)
	}
	if raws != 0 {
		t.Errorf("raw endpoint hit %d times during the startup check", raws)
	}
}

// TestLatestErrorsWithoutTags: an empty tag list is a resolution failure,
// never a silent "version unknown that looks like success".
func TestLatestErrorsWithoutTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	if _, err := NewWithBaseURLs(server.Client(), server.URL, server.URL).Latest(context.Background()); err == nil {
		t.Fatal("expected an error for an empty tag list")
	}
}

// TestCachedVersionReadsNewestCompleteCacheSet: the startup cached-half is
// local-only — complete commit dirs report their nvngx_dlss.dll PE version,
// newest wins; incomplete or absent sets report nothing.
func TestCachedVersionReadsNewestCompleteCacheSet(t *testing.T) {
	cache := t.TempDir()
	if v, commit := CachedVersion(cache); v != "" || commit != "" {
		t.Fatalf("empty cache = %q/%q, want empty", v, commit)
	}
	seedCacheDir(t, cache, strings.Repeat("a", 40), 310, 6)
	seedCacheDir(t, cache, strings.Repeat("b", 40), 310, 9)
	if v, commit := CachedVersion(cache); v != "310.9.0.0" || commit != strings.Repeat("b", 40) {
		t.Fatalf("cached = %q/%q, want 310.9.0.0 @ b-padded commit", v, commit)
	}
	// An incomplete set (missing member) is invisible to the check.
	incomplete := filepath.Join(cache, "dlss", strings.Repeat("c", 40))
	if err := os.MkdirAll(incomplete, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := cacheRecord{Files: map[string]string{Files[0]: "h0", Files[1]: "h1"}}
	if err := jsoncache.Write(filepath.Join(incomplete, "manifest.json"), rec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, Files[0]), testutil.FixedVersionPE(311, 0, 0, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, commit := CachedVersion(cache); v != "310.9.0.0" || commit != strings.Repeat("b", 40) {
		t.Fatalf("after incomplete dir = %q/%q, want 310.9.0.0 @ b-padded commit", v, commit)
	}
}

// seedCacheDir plants one complete commit-keyed cache dir with a manifest
// pinning the real member digests.
func seedCacheDir(t *testing.T, cacheRoot, commit string, maj, min uint16) {
	t.Helper()
	dir := filepath.Join(cacheRoot, "dlss", commit)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for i, name := range Files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, testutil.FixedVersionPE(maj, min, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
		h, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = h
	}
	if err := jsoncache.Write(filepath.Join(dir, "manifest.json"), cacheRecord{Files: files}); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateUsesCommitHintWithoutNetwork: when the startup check's published
// commit is already in the cache, pressing the DLSS badge installs straight
// from the cache with zero network — the "fetch from cache if latest, else
// download" contract. An unknown hint falls back to the online resolve.
func TestUpdateUsesCommitHintWithoutNetwork(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 1, 0)
	hint := strings.Repeat("a", 40)
	seedCacheDir(t, cache, hint, 310, 9)

	commits, raws := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/NVIDIA/DLSS/commits/main":
			commits++
			http.Error(w, "cache hit must not resolve online", http.StatusInternalServerError)
		default:
			raws++
			http.Error(w, "cache hit must not download", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), cache, root, game, hint)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceCommit != hint {
		t.Fatalf("source commit %q, want the cached hint %q", snap.SourceCommit, hint)
	}
	if commits != 0 || raws != 0 {
		t.Errorf("cache-hit update hit the network (commits %d, raws %d)", commits, raws)
	}
	assertGameVersion(t, game, "310.9.0.0")

	// A hint whose cache dir does not exist falls back to the online
	// resolve + download.
	missing := strings.Repeat("b", 40)
	var rawsFallback int
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/NVIDIA/DLSS/commits/main":
			_, _ = w.Write([]byte(`{"sha":"` + strings.Repeat("c", 40) + `"}`))
		default:
			rawsFallback++
			_, _ = w.Write(testutil.FixedVersionPE(310, 10, 0, 0))
		}
	}))
	defer server2.Close()
	if _, err := Update(context.Background(), NewWithBaseURLs(server2.Client(), server2.URL, server2.URL), cache, root, game, missing); err != nil {
		t.Fatal(err)
	}
	if rawsFallback == 0 {
		t.Errorf("cache-miss hint fetched no runtime files (commits resolved, downloads %d)", rawsFallback)
	}
	assertGameVersion(t, game, "310.10.0.0")
}

// assertGameVersion reads the game's DLSS version, failing on any error.
func assertGameVersion(t *testing.T, game, want string) {
	t.Helper()
	v, err := fileVersion(filepath.Join(game, "nvngx_dlss.dll"))
	if err != nil || v != want {
		t.Fatalf("game DLSS version = %q (err %v), want %q", v, err, want)
	}
}

// TestUpdateAlreadyLatestIsNoOp: pressing the DLSS badge must compare the
// target against the installed set — byte-identical members (the cache's
// own digests), or a readable applied version at or above the target — and
// refuse the reinstall and its backup gracefully.
func TestUpdateAlreadyLatestIsNoOp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		applied [2]uint16
		cached  [2]uint16
		want    string
		stays   string // the applied version the no-op must leave untouched
	}{
		{"byte-identical set", [2]uint16{310, 9}, [2]uint16{310, 9}, "310.9.0.0", "310.9.0.0"},
		{"applied newer than cache", [2]uint16{311, 0}, [2]uint16{310, 9}, "311.0.0.0", "311.0.0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "game")
			cache := filepath.Join(root, "cache")
			if err := os.MkdirAll(game, 0o755); err != nil {
				t.Fatal(err)
			}
			seedGame(t, game, tc.applied[0], tc.applied[1])
			hint := strings.Repeat("a", 40)
			seedCacheDir(t, cache, hint, tc.cached[0], tc.cached[1])

			commits, raws := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/NVIDIA/DLSS/commits/main":
					commits++
				default:
					raws++
				}
				http.Error(w, "already-latest press must not touch the network", http.StatusInternalServerError)
			}))
			defer server.Close()

			_, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), cache, root, game, hint)
			var already *AlreadyLatestError
			if !errors.As(err, &already) {
				t.Fatalf("update err = %v, want AlreadyLatestError", err)
			}
			if already.Version != tc.want {
				t.Errorf("already-latest version = %q, want %q", already.Version, tc.want)
			}
			assertGameVersion(t, game, tc.stays)
			if commits != 0 || raws != 0 {
				t.Errorf("no-op press hit the network (commits %d, raws %d)", commits, raws)
			}
			if snaps, _ := Snapshots(root, game); len(snaps) != 0 {
				t.Errorf("no-op press created %d snapshots", len(snaps))
			}
		})
	}
}

// TestUpdateRestoreBackupsDeduplicated: a backup whose member digests an
// existing snapshot already holds is reused, not duplicated — the
// update/restore ping-pong must not pile up identical ~115 MB dirs.
func TestUpdateRestoreBackupsDeduplicated(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	seedGame(t, game, 310, 5)
	seedCacheDir(t, cache, strings.Repeat("a", 40), 310, 6)
	seedCacheDir(t, cache, strings.Repeat("b", 40), 310, 7)

	commits, raws := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			commits++
		} else {
			raws++
		}
		http.Error(w, "cached updates must not touch the network", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	update := func(hint string) {
		t.Helper()
		if _, err := Update(context.Background(), client, cache, root, game, hint); err != nil {
			t.Fatal(err)
		}
	}
	updateAlreadyLatest := func(hint, want string) {
		t.Helper()
		_, err := Update(context.Background(), client, cache, root, game, hint)
		var already *AlreadyLatestError
		if !errors.As(err, &already) || already.Version != want {
			t.Fatalf("update err = %v, want AlreadyLatestError{%s}", err, want)
		}
	}
	restore := func(id string) {
		t.Helper()
		if _, err := Restore(context.Background(), root, game, id); err != nil {
			t.Fatal(err)
		}
	}
	snapAt := func(v string) string {
		t.Helper()
		snaps, err := Snapshots(root, game)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snaps {
			if s.Files[0].Version == v {
				return s.ID
			}
		}
		t.Fatalf("no snapshot holding %s in %+v", v, snaps)
		return ""
	}
	assertSnapshots := func(want ...string) {
		t.Helper()
		snaps, err := Snapshots(root, game)
		if err != nil {
			t.Fatal(err)
		}
		if len(snaps) != len(want) {
			t.Fatalf("snapshots %d (%+v), want %d versions %v", len(snaps), snaps, len(want), want)
		}
		seen := map[string]bool{}
		for _, s := range snaps {
			seen[s.Files[0].Version] = true
		}
		for _, v := range want {
			if !seen[v] {
				t.Errorf("missing snapshot holding %s", v)
			}
		}
	}

	update(strings.Repeat("a", 40)) // backs up 310.5 → S1, installs 310.6
	assertSnapshots("310.5.0.0")
	update(strings.Repeat("b", 40)) // backs up 310.6 → S2, installs 310.7
	assertSnapshots("310.5.0.0", "310.6.0.0")
	restore(snapAt("310.5.0.0")) // backs up 310.7 → S3, back to 310.5
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	restore(snapAt("310.5.0.0")) // current set == target: its backup is S1, reused
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	update(strings.Repeat("a", 40)) // backs up 310.5 → S1 again (reused), installs 310.6
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	updateAlreadyLatest(strings.Repeat("a", 40), "310.6.0.0")
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	update(strings.Repeat("b", 40)) // backs up 310.6 → S2 again (reused), installs 310.7
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	updateAlreadyLatest(strings.Repeat("b", 40), "310.7.0.0")
	assertSnapshots("310.5.0.0", "310.6.0.0", "310.7.0.0")
	assertGameVersion(t, game, "310.7.0.0")

	// A tampered prior snapshot is not a dedup candidate: its recorded
	// digests match the installed set, but its stored bytes must re-verify
	// before reuse — the identical press writes a fresh, self-verified
	// backup instead of aliasing corrupted bytes.
	s3 := snapAt("310.7.0.0")
	if err := os.WriteFile(filepath.Join(snapshotsDir(root, game), s3, Files[0]), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore(snapAt("310.5.0.0")) // backs up current 310.7 fresh (S3 unusable), then restores S1
	assertGameVersion(t, game, "310.5.0.0")
	update(strings.Repeat("a", 40)) // S1 (310.5, intact) is still reused; installs 310.6
	assertGameVersion(t, game, "310.6.0.0")

	snaps, err := Snapshots(root, game)
	if err != nil {
		t.Fatal(err)
	}
	byVersion := map[string]int{}
	for _, s := range snaps {
		byVersion[s.Files[0].Version]++
	}
	if len(snaps) != 4 || byVersion["310.5.0.0"] != 1 || byVersion["310.6.0.0"] != 1 || byVersion["310.7.0.0"] != 2 {
		t.Fatalf("snapshots after tamper (count %d): %+v — want one 310.5, one 310.6, two 310.7 (fresh backup written)", len(snaps), snaps)
	}
	if commits != 0 || raws != 0 {
		t.Errorf("cached sequence hit the network (commits %d, raws %d)", commits, raws)
	}
}
