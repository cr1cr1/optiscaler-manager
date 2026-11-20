package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// dlssTestServer serves one fixed-version NVIDIA runtime set: commits/main
// resolves to a fixed SHA, every runtime member downloads as that PE.
func dlssTestServer(t *testing.T, maj, min, patch, build uint16) (*httptest.Server, string) {
	t.Helper()
	sha := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/NVIDIA/DLSS/commits/main":
			_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
		case strings.Contains(r.URL.Path, "/"+sha+"/lib/Windows_x86_64/rel/"):
			_, _ = w.Write(testutil.FixedVersionPE(maj, min, patch, build))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, sha
}

func seedDLSSSet(t *testing.T, dir string, maj, min uint16) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, name := range dlss.Files {
		if err := os.WriteFile(filepath.Join(dir, name), testutil.FixedVersionPE(maj, min, uint16(i), 0), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestUpdateDLSSTargetsOptiScalerDir: with OptiScaler installed, the
// NVIDIA runtime set goes where dlssg_to_fsr3_amd_is_better.dll lives —
// the OptiScaler/ subdir for the dlssnr fork — even though the resolved
// injection dir is the game root. Backups live beside the DLLs they
// protect, and nothing lands in the game root (issue 081).
func TestUpdateDLSSTargetsOptiScalerDir(t *testing.T) {
	game := t.TempDir()
	writeScanFile(t, filepath.Join(game, "game.exe"), []byte("GAME"))
	opti := filepath.Join(game, "OptiScaler")
	writeScanFile(t, filepath.Join(opti, "dlssg_to_fsr3_amd_is_better.dll"), []byte("FG"))
	seedDLSSSet(t, opti, 1, 0)

	server, _ := dlssTestServer(t, 310, 9, 1, 0)
	client := dlss.NewWithBaseURLs(server.Client(), server.URL, server.URL)
	if _, err := UpdateDLSS(context.Background(), client, t.TempDir(), game, "", false); err != nil {
		t.Fatalf("UpdateDLSS: %v", err)
	}
	for _, name := range dlss.Files {
		v, err := pever.FileVersion(filepath.Join(opti, name))
		if err != nil || v != "310.9.1.0" {
			t.Errorf("OptiScaler/%s version %q, err=%v; want 310.9.1.0", name, v, err)
		}
		if _, err := os.Stat(filepath.Join(game, name)); !os.IsNotExist(err) {
			t.Errorf("%s written to the game root (err=%v); want it beside dlssg_to_fsr3", name, err)
		}
	}
	backups, err := os.ReadDir(filepath.Join(opti, "dlss-backups"))
	if err != nil || len(backups) == 0 {
		t.Fatalf("no backup under OptiScaler/dlss-backups (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(game, "dlss-backups")); !os.IsNotExist(err) {
		t.Errorf("backup written to the game root (err=%v); want it beside the DLLs", err)
	}
}

// TestUpdateDLSSLeavesShadowSetUntouched: on a shadow layout the update
// manages the OptiScaler-dir set and never touches the stranded root
// copy — no automatic deletion, move, or overwrite of game files
// without consent (issue 083).
func TestUpdateDLSSLeavesShadowSetUntouched(t *testing.T) {
	game := t.TempDir()
	writeScanFile(t, filepath.Join(game, "game.exe"), []byte("GAME"))
	seedDLSSSet(t, game, 3, 7)
	opti := filepath.Join(game, "OptiScaler")
	writeScanFile(t, filepath.Join(opti, "dlssg_to_fsr3_amd_is_better.dll"), []byte("FG"))
	seedDLSSSet(t, opti, 1, 0)

	before := map[string]string{}
	for _, name := range dlss.Files {
		h, err := fileSHA256(filepath.Join(game, name))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = h
	}

	server, _ := dlssTestServer(t, 310, 9, 1, 0)
	client := dlss.NewWithBaseURLs(server.Client(), server.URL, server.URL)
	if _, err := UpdateDLSS(context.Background(), client, t.TempDir(), game, "", false); err != nil {
		t.Fatalf("UpdateDLSS: %v", err)
	}
	for _, name := range dlss.Files {
		h, err := fileSHA256(filepath.Join(game, name))
		if err != nil || h != before[name] {
			t.Errorf("shadow root %s changed (err=%v); the updater must not touch it", name, err)
		}
		if v, err := pever.FileVersion(filepath.Join(opti, name)); err != nil || v != "310.9.1.0" {
			t.Errorf("OptiScaler/%s version %q, err=%v; want the update applied there", name, v, err)
		}
	}
}

// TestResolveDLSSDirPrefersAnchorOverBackups: the anchor search skips
// rollback stores (an uninstalled OptiScaler's relocated copy must not
// hijack the target) and breaks anchor ties deterministically; without
// any anchor the injection dir stands (issue 081).
func TestResolveDLSSDirPrefersAnchorOverBackups(t *testing.T) {
	mk := func(t *testing.T) string {
		t.Helper()
		game := t.TempDir()
		writeScanFile(t, filepath.Join(game, "game.exe"), []byte("GAME"))
		root, err := filepath.EvalSymlinks(game)
		if err != nil {
			t.Fatal(err)
		}
		return root
	}

	anchor := mk(t)
	writeScanFile(t, filepath.Join(anchor, "OptiScaler", "dlssg_to_fsr3_amd_is_better.dll"), []byte("FG"))
	writeScanFile(t, filepath.Join(anchor, "optiscaler-backups", "dlssg_to_fsr3_amd_is_better.dll"), []byte("STALE"))
	if got, err := ResolveDLSSDir(anchor); err != nil || got != filepath.Join(anchor, "OptiScaler") {
		t.Errorf("anchor dir = %q, err=%v; want the live OptiScaler subdir", got, err)
	}

	tied := mk(t)
	writeScanFile(t, filepath.Join(tied, "b", "dlssg_to_fsr3_amd_is_better.dll"), []byte("FG"))
	writeScanFile(t, filepath.Join(tied, "a", "dlssg_to_fsr3_amd_is_better.dll"), []byte("FG"))
	if got, err := ResolveDLSSDir(tied); err != nil || got != filepath.Join(tied, "a") {
		t.Errorf("tied anchors = %q, err=%v; want the smallest path", got, err)
	}

	plain := mk(t)
	if got, err := ResolveDLSSDir(plain); err != nil || got != plain {
		t.Errorf("no-anchor dir = %q, err=%v; want the injection dir", got, err)
	}
}

// dlssg_to_fsr3 anywhere) the update keeps today's behavior — the
// resolved injection dir is the destination (issue 081 guard).
// TestUpdateDLSSFallsBackToInjectionDir: without OptiScaler (no
func TestUpdateDLSSFallsBackToInjectionDir(t *testing.T) {
	game := t.TempDir()
	writeScanFile(t, filepath.Join(game, "game.exe"), []byte("GAME"))
	seedDLSSSet(t, game, 1, 0)

	server, _ := dlssTestServer(t, 310, 9, 1, 0)
	client := dlss.NewWithBaseURLs(server.Client(), server.URL, server.URL)
	if _, err := UpdateDLSS(context.Background(), client, t.TempDir(), game, "", false); err != nil {
		t.Fatalf("UpdateDLSS: %v", err)
	}
	for _, name := range dlss.Files {
		v, err := pever.FileVersion(filepath.Join(game, name))
		if err != nil || v != "310.9.1.0" {
			t.Errorf("%s version %q, err=%v; want 310.9.1.0", name, v, err)
		}
	}
}
