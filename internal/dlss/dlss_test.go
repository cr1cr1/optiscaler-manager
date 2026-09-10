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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}

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

	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game)
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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}

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

	first, err := Update(context.Background(), client, cache, root, game)
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

	// Second update of the same commit: the network is closed for business.
	refuse = true
	if _, err := Update(context.Background(), client, cache, root, game); err != nil {
		t.Fatalf("cached update failed: %v", err)
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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sha := strings.Repeat("f", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	client := NewWithBaseURLs(server.Client(), server.URL, server.URL)

	if _, err := Update(context.Background(), client, cache, root, game); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(cache, "dlss", sha, Files[0])
	if err := os.WriteFile(tampered, testutil.FixedVersionPE(9, 9, 9, 9), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(context.Background(), client, cache, root, game); err != nil {
		t.Fatalf("update with tampered cache entry failed: %v", err)
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

func TestRestoreRestoresCompletePriorSnapshot(t *testing.T) {
	root, game := t.TempDir(), ""
	game = filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sha := strings.Repeat("b", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	first, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game)
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

func TestUpdateRefusesWhenAnyNVIDIADLLIsMissing(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, Files[0]), testutil.FixedVersionPE(1, 0, 0, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(context.Background(), nil, root, root, game); err == nil || !strings.Contains(err.Error(), Files[1]) {
		t.Fatalf("err %v, want missing %s", err, Files[1])
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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sha := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	first, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game)
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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(2, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Update(ctx, New(nil), root, root, game); !errors.Is(err, context.Canceled) {
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
	for i, name := range Files {
		if err := os.WriteFile(filepath.Join(game, name), testutil.FixedVersionPE(1, 0, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sha := strings.Repeat("c", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/NVIDIA/DLSS/commits/main" {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
			return
		}
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	}))
	defer server.Close()
	snap, err := Update(context.Background(), NewWithBaseURLs(server.Client(), server.URL, server.URL), root, root, game)
	if err != nil {
		t.Fatal(err)
	}
	return root, game, snap
}

func assertGameUnchanged(t *testing.T, game, want string) {
	t.Helper()
	v, err := fileVersion(filepath.Join(game, "nvngx_dlss.dll"))
	if err != nil || v != want {
		t.Fatalf("game DLL changed to %q (err %v), want %q", v, err, want)
	}
}
