package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// TestStartPreloadsLatestBundleIntoCache (user spec, bug report): "at
// program startup, optiscaler and nvidia DLSS dlls versions are checked,
// and `latest` downloaded in the cache directory". Repro state mirrors the
// user's machine: a WARM games cache (one game committed at the latest
// tag), the default version preference at "latest" with online lookups on,
// and a COLD download cache — nothing pre-fetched yet.
//
// Expected: Start settles with the latest bundle present in the download
// cache (cacheDir/optiscaler/<latest-tag>/*.7z), so the version dropdown
// offers it as an installable, offline-ready option. (Written red first:
// the startup only RESOLVED the tag and downloaded nothing.)
func TestStartPreloadsLatestBundleIntoCache(t *testing.T) {
	e := newTestEnv(t)
	root := t.TempDir()
	e.sess.deps.SettingsRoot = root
	e.sess.deps.Settings = settings.Defaults() // OnlineLookups on, default "latest"

	// Warm games cache: one committed row at the latest tag (the user's
	// Witcher 3 row shape), so Start takes the warm-boot path, not a scan.
	saveGamesCache(root, []GameRow{{
		Title:             "Game One",
		InstallDir:        e.gameRoot,
		InjectionDir:      e.bin,
		Platform:          domain.StoreSteam.String(),
		Store:             domain.StoreSteam,
		Status:            domain.StatusCommitted,
		OptiScalerVersion: "v0.9.4-test",
	}})

	e.sess.Start(context.Background())
	if st := e.sess.Snapshot(); st.StatusLine != "1 games (cached)" {
		t.Fatalf("StatusLine = %q, want warm-cache boot", st.StatusLine)
	}

	// The latest bundle must land in the download cache at startup.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(e.sess.deps.CacheDir, "optiscaler", "v0.9.4-test", "*.7z"))
		if len(matches) > 0 {
			t.Log("startup preloaded the latest bundle into the cache:", matches[0])
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("startup did not download the latest bundle into %q (the dropdown can only offer old/cached versions)", e.sess.deps.CacheDir)
}

// TestStartPreloadSkippedWhenBundleCached (user spec: pre-download `latest`
// "in the cache directory", skipped when already cached): with the latest
// bundle ALREADY in the download cache (the user's own state), startup
// resolves the tag but fetches ZERO bundle bytes — the pre-warm is a
// fill-missing, not a refetch.
func TestStartPreloadSkippedWhenBundleCached(t *testing.T) {
	e := newTestEnv(t)
	root := t.TempDir()
	e.sess.deps.SettingsRoot = root
	e.sess.deps.Settings = settings.Defaults()

	saveGamesCache(root, []GameRow{{
		Title:             "Game One",
		InstallDir:        e.gameRoot,
		InjectionDir:      e.bin,
		Platform:          domain.StoreSteam.String(),
		Store:             domain.StoreSteam,
		Status:            domain.StatusCommitted,
		OptiScalerVersion: "v0.9.4-test",
	}})
	// The latest bundle is pre-cached (the user's v0.9.4 state).
	bundle, err := os.ReadFile(filepath.Join("..", "installer", "testdata", "bundle.7z"))
	if err != nil {
		t.Fatal(err)
	}
	bdir := filepath.Join(e.sess.deps.CacheDir, "optiscaler", "v0.9.4-test")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bdir, "Optiscaler_test.7z"), bundle, 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())

	// Wait for the preload to run (memo set: the resolve happened, and the
	// skip-or-download decision lands immediately after).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && e.sess.LatestKnown() == "" {
		time.Sleep(20 * time.Millisecond)
	}
	if got := e.sess.LatestKnown(); got != "v0.9.4-test" {
		t.Fatalf("LatestKnown = %q, want the startup-resolved latest tag", got)
	}
	if hits := atomic.LoadInt64(&e.bundleHits); hits != 0 {
		t.Errorf("startup pre-warm fetched the bundle %d times although it was already cached", hits)
	}
	t.Log("already-cached latest bundle: resolved, not re-fetched")
}

// TestSwitchVersionLatestResolvesAtPickTime (user spec): picking the
// "Latest" menu option dispatches the literal "latest" — the switch
// re-resolves the newest release at pick time (never the possibly-stale
// startup memo) and lands at that concrete tag.
func TestSwitchVersionLatestResolvesAtPickTime(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e) // committed at the older tag

	e.sess.SwitchVersion(e.gameRoot, "latest")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg first", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want the install leg second", ev.Text)
	}

	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.10.0-test" {
		t.Errorf("manifest version = %q, want v0.10.0-test (the latest, resolved at pick time)", manifests[0].Resolved.Version)
	}
	t.Log("switch to \"latest\" re-resolved at pick time and landed at the newest release")
}

// TestSwitchVersionLatestSameVersionNoOp: switching to "latest" when the
// game is ALREADY at the latest runs no op — the resolved tag equals the
// installed version, so no uninstall churn, no mid-chain events. The only
// event is the single EvOpSettled "already at" report the CLI's one-shot
// waiter needs (the frontends only poke on events).
func TestSwitchVersionLatestSameVersionNoOp(t *testing.T) {
	e := newUpgradeEnv(t, "latest")
	installAt(t, e) // default "latest" installs v0.10.0-test

	e.sess.SwitchVersion(e.gameRoot, "latest")
	select {
	case ev := <-e.sess.Events():
		if ev.Kind != EvOpSettled || ev.Text != "already at v0.10.0-test" {
			t.Fatalf("latest switch on the latest fired %v %q, want one EvOpSettled \"already at v0.10.0-test\"", ev.Kind, ev.Text)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("latest no-op switch never settled (a CLI waiter would hang)")
	}
	row := theRow(t, e.sess)
	if row.Status != domain.StatusCommitted || row.OptiScalerVersion != "v0.10.0-test" {
		t.Errorf("row after the latest no-op = %+v, want committed at v0.10.0-test", row)
	}
	t.Log("switching to the latest while already at the latest was a reported no-op")
}
