package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// writeCachedBundle plants a bundle-cache fixture exactly where
// app.CachedVersions looks: <cacheDir>/optiscaler/<tag>/Optiscaler_*.7z.
func writeCachedBundle(t *testing.T, cacheDir, tag string) {
	t.Helper()
	dir := filepath.Join(cacheDir, "optiscaler", tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Optiscaler_"+tag+".7z"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write bundle fixture: %v", err)
	}
}

func countOccurrences(haystack []string, needle string) int {
	n := 0
	for _, v := range haystack {
		if v == needle {
			n++
		}
	}
	return n
}

// assertSortedDesc fails unless got is ordered newest-first by
// version.Compare — the dropdown/cycling order the frontends rely on.
func assertSortedDesc(t *testing.T, got []string) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if version.Compare(got[i-1], got[i]) <= 0 {
			t.Fatalf("Versions not semver-descending at %d: %v", i, got)
		}
	}
}

// TestVersionsOnlineLatestContributesResolvedTag (S3): with the preference
// at "latest" and a successful resolution, the list carries the RESOLVED
// concrete tag — the literal string "latest" must never be offered as an
// installable version. The row's installed version and cached bundles join
// the union (deduped exactly) and the whole list sorts newest first.
func TestVersionsOnlineLatestContributesResolvedTag(t *testing.T) {
	e := newUpgradeEnv(t, "latest")
	installAt(t, e) // row committed at v0.10.0-test; memo resolved by the scan
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "v0.10.0-test") != 1 {
		t.Errorf("Versions = %v, want v0.10.0-test exactly once (installed = resolved default, deduped)", got)
	}
	if countOccurrences(got, "v0.9.4-test") != 1 {
		t.Errorf("Versions = %v, want cached v0.9.4-test exactly once", got)
	}
	if countOccurrences(got, "latest") != 0 {
		t.Errorf("Versions = %v, must never contain the literal \"latest\"", got)
	}
	assertSortedDesc(t, got)
	if len(got) == 0 || got[0] != "v0.10.0-test" {
		t.Errorf("Versions = %v, want newest (v0.10.0-test) first", got)
	}
}

// TestVersionsOfflineLatestNeverResolves (S4): "latest" is unresolvable
// offline, so the memo is empty and the preference contributes NOTHING —
// the list is exactly installed ∪ cached. Versions must not touch the
// resolver seam (or the network) to find out: it reads the memo and the
// filesystem only.
func TestVersionsOfflineLatestNeverResolves(t *testing.T) {
	e := newUpgradeEnvLookups(t, "latest", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.8.0")
	scanAndWait(t, e.sess)
	if got := e.resolves.Load(); got != 0 {
		t.Fatalf("precondition: offline scan resolved %d times, want 0", got)
	}

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	want := []string{"v0.9.4-test", "v0.8.0"}
	if len(got) != len(want) {
		t.Fatalf("Versions = %v, want %v (cached only; unresolved latest contributes nothing)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Versions = %v, want %v", got, want)
		}
	}
	if got := e.resolves.Load(); got != 0 {
		t.Fatalf("Versions triggered %d resolver calls, want 0 (offline rule: memo + filesystem only)", got)
	}
}

// TestVersionsPinnedPreferenceIncludedOffline (S5): a pinned concrete
// preference needs no resolution, so it is offered verbatim — even when no
// bundle is cached for it and the session is offline.
func TestVersionsPinnedPreferenceIncludedOffline(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.8.0", false) // not in the fake releases, not cached
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")
	scanAndWait(t, e.sess)

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "v0.8.0") != 1 {
		t.Errorf("Versions = %v, want pinned preference v0.8.0 exactly once (uncached, offline)", got)
	}
	if countOccurrences(got, "v0.9.4-test") != 1 {
		t.Errorf("Versions = %v, want cached v0.9.4-test exactly once", got)
	}
	assertSortedDesc(t, got)
	if got := e.resolves.Load(); got != 0 {
		t.Fatalf("Versions triggered %d resolver calls, want 0", got)
	}
}

// TestVersionsIncludesMixedFormatsVerbatim: installed version evidence can
// be a bare PE-probe version ("0.7.9") while cached tags are v-prefixed.
// Dedupe is exact-string only — NO cross-format normalization — so both
// forms may legitimately coexist; each appears verbatim, sorted by
// version.Compare (which already equates "0.7.9" and "v0.7.9" for ORDER).
func TestVersionsIncludesMixedFormatsVerbatim(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.9.4-test", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")
	scanAndWait(t, e.sess)
	e.sess.setRowInstalled(e.gameRoot, "0.7.9") // bare PE-probe style

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "0.7.9") != 1 {
		t.Errorf("Versions = %v, want installed 0.7.9 verbatim exactly once", got)
	}
	if countOccurrences(got, "v0.9.4-test") != 1 {
		t.Errorf("Versions = %v, want v0.9.4-test exactly once (installed+cached+pref dedupe)", got)
	}
	assertSortedDesc(t, got)
}

// TestVersionsDedupeSemanticPrefersInstalled (B1): an installed bare
// PE-probe version ("0.9.4") and a cached v-prefixed tag ("v0.9.4") are the
// SAME version (version.Compare == 0), so the list must carry exactly ONE
// entry — the INSTALLED form. The frontends' tick and same-version no-op
// are exact-string against the row's OptiScalerVersion; keeping the
// installed form verbatim keeps the dropdown ticked and re-selection a
// no-op. The cached duplicate must not appear.
func TestVersionsDedupeSemanticPrefersInstalled(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.8.0", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4")
	scanAndWait(t, e.sess)
	e.sess.setRowInstalled(e.gameRoot, "0.9.4") // bare PE-probe style

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "0.9.4") != 1 {
		t.Errorf("Versions = %v, want installed form \"0.9.4\" exactly once", got)
	}
	if countOccurrences(got, "v0.9.4") != 0 {
		t.Errorf("Versions = %v, semantic duplicate \"v0.9.4\" must not appear (installed form wins)", got)
	}
	assertSortedDesc(t, got)
}

// TestVersionsDedupeSemanticPrefersInstalledVForm (B1, mirrored): the
// installed evidence itself can be v-prefixed (e.g. adopted from a
// manifest) while a cache directory is literally named "0.9.4" — again one
// version, and again the INSTALLED form ("v0.9.4") survives.
func TestVersionsDedupeSemanticPrefersInstalledVForm(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.8.0", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "0.9.4") // bare-named cache dir
	scanAndWait(t, e.sess)
	e.sess.setRowInstalled(e.gameRoot, "v0.9.4")

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "v0.9.4") != 1 {
		t.Errorf("Versions = %v, want installed form \"v0.9.4\" exactly once", got)
	}
	if countOccurrences(got, "0.9.4") != 0 {
		t.Errorf("Versions = %v, semantic duplicate \"0.9.4\" must not appear (installed form wins)", got)
	}
	assertSortedDesc(t, got)
}

// TestVersionsDedupeKeepsPrereleaseDistinct (B1 guard): a pre-release is a
// DIFFERENT version from the plain release (version.Compare != 0), so
// installed "0.9.4" and cached "v0.9.4-test" must BOTH stay in the list —
// semantic dedupe must not over-collapse.
func TestVersionsDedupeKeepsPrereleaseDistinct(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.8.0", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")
	scanAndWait(t, e.sess)
	e.sess.setRowInstalled(e.gameRoot, "0.9.4")

	got := e.sess.Versions(e.gameRoot)
	t.Logf("Versions = %v", got)
	if countOccurrences(got, "0.9.4") != 1 {
		t.Errorf("Versions = %v, want installed \"0.9.4\" exactly once", got)
	}
	if countOccurrences(got, "v0.9.4-test") != 1 {
		t.Errorf("Versions = %v, want prerelease \"v0.9.4-test\" exactly once (a different version, not a duplicate)", got)
	}
	assertSortedDesc(t, got)
}

// TestVersionsUnknownGameDir: a defensive call for a dir with no row still
// gets the row-independent part of the list (cached ∪ preference) — the
// same data a row-less dropdown needs. Documented behavior, not an error.
func TestVersionsUnknownGameDir(t *testing.T) {
	e := newUpgradeEnvLookups(t, "v0.8.0", false)
	writeCachedBundle(t, e.sess.deps.CacheDir, "v0.9.4-test")
	scanAndWait(t, e.sess)

	got := e.sess.Versions(filepath.Join(e.gameRoot, "no-such-game"))
	t.Logf("Versions(unknown) = %v", got)
	want := []string{"v0.9.4-test", "v0.8.0"}
	if len(got) != len(want) {
		t.Fatalf("Versions(unknown) = %v, want %v (cached ∪ preference, no installed entry)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Versions(unknown) = %v, want %v", got, want)
		}
	}
}

// TestVersionsEmptyEverywhere: no row, empty cache, unresolved "latest" —
// the contract is an empty (non-nil) slice, so frontends can range over the
// result without a nil check.
func TestVersionsEmptyEverywhere(t *testing.T) {
	sess := NewSession(Deps{}) // DefaultVersion defaults to "latest", no memo
	got := sess.Versions("/anything")
	if got == nil {
		t.Fatal("Versions returned nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("Versions = %v, want empty (no row, no cache, unresolved latest)", got)
	}
}
