package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

func sha(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newGame builds a fake game root with a bin/ dir holding the game exe and
// returns (gameRoot, installDir, store).
func newGame(t *testing.T) (string, string, *store.Store) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	writeFile(t, filepath.Join(bin, "game.exe"), "GAME-EXE")
	return root, bin, store.New(t.TempDir())
}

func request(gameRoot, installDir string) Request {
	return Request{
		GameRoot:         gameRoot,
		InstallDir:       installDir,
		ArchivePath:      filepath.Join("testdata", "bundle.7z"),
		RequestedVersion: "latest",
		Resolved:         domain.ResolvedAsset{AssetName: "Optiscaler_test.7z", Version: "v0.0.0-test"},
	}
}

func manifestID(t *testing.T, installDir string) string {
	t.Helper()
	c, err := canonicalPath(installDir)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	return domain.ManifestID(c)
}

func TestRejectsUnsafeArchive(t *testing.T) {
	bad := [][]string{
		{"../evil.dll", "fakenvapi.dll", "fakenvapi.ini"},
		{"/abs/optiscaler.dll", "fakenvapi.dll", "fakenvapi.ini"},
		{"optiscaler.dll", "fakenvapi.dll", "fakenvapi.ini", "sub/../../evil"},
		{"optiscaler.dll", "fakenvapi.dll", "fakenvapi.ini", "DXGI.dll", "dxgi.dll"}, // collides with injection rename
		{"fakenvapi.dll", "fakenvapi.ini"},                                           // missing optiscaler.dll
	}
	for _, names := range bad {
		if _, err := buildPlan(names); err == nil {
			t.Errorf("buildPlan(%v): expected rejection", names)
		} else {
			t.Logf("rejected %v: %v", names, err)
		}
	}

	good := []string{"OptiScaler.dll", "fakenvapi.dll", "fakenvapi.ini", "libxess.dll", "D3D12_Optiscaler/", "D3D12_Optiscaler/D3D12Core.dll"}
	plan, err := buildPlan(good)
	if err != nil {
		t.Fatalf("buildPlan(good): %v", err)
	}
	// OptiScaler.dll must be renamed to the injection DLL; dirs skipped.
	byDst := map[string]string{}
	for _, fp := range plan {
		byDst[fp.dstRel] = fp.srcRel
	}
	if byDst["dxgi.dll"] != "OptiScaler.dll" {
		t.Errorf("injection rename missing: plan=%v", plan)
	}
	if _, ok := byDst[filepath.Join("D3D12_Optiscaler", "D3D12Core.dll")]; !ok {
		t.Errorf("nested file missing: plan=%v", plan)
	}
	for _, fp := range plan {
		if strings.HasSuffix(fp.srcRel, "/") {
			t.Errorf("directory entry leaked into plan: %v", fp)
		}
	}
	t.Logf("plan: %v", plan)
}

func TestBacksUpBeforeOverwrite(t *testing.T) {
	root, bin, st := newGame(t)
	original := "ORIGINAL-GAME-DXGI-BYTES"
	writeFile(t, filepath.Join(bin, "dxgi.dll"), original)

	m, err := Install(context.Background(), st, request(root, bin))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	id := manifestID(t, bin)
	backup := filepath.Join(st.BackupDir(id), "files", "dxgi.dll")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup missing at %s: %v", backup, err)
	}
	if string(data) != original {
		t.Fatalf("backup bytes %q != original %q", data, original)
	}

	var ow *domain.OverwrittenEntry
	for i := range m.Overwritten {
		if filepath.Base(m.Overwritten[i].Path) == "dxgi.dll" {
			ow = &m.Overwritten[i]
		}
	}
	if ow == nil {
		t.Fatalf("no overwritten entry for dxgi.dll: %+v", m.Overwritten)
	}
	if ow.PreSHA256 != sha(t, backup) {
		t.Errorf("PreSHA256 %q != backup hash", ow.PreSHA256)
	}
	if ow.InstalledSHA256 != sha(t, filepath.Join(bin, "dxgi.dll")) {
		t.Errorf("InstalledSHA256 mismatch with installed bytes")
	}
	t.Logf("backup verified: %s, pre=%s installed=%s", backup, ow.PreSHA256, ow.InstalledSHA256)
}

func TestRecordsCreatedAndOverwritten(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "OptiScaler.ini"), "ORIGINAL-INI")

	m, err := Install(context.Background(), st, request(root, bin))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if m.Status != domain.StatusCommitted {
		t.Fatalf("status %q, want committed", m.Status)
	}

	var owPaths, crPaths []string
	for _, e := range m.Overwritten {
		owPaths = append(owPaths, filepath.Base(e.Path))
	}
	for _, e := range m.Created {
		crPaths = append(crPaths, filepath.Base(e.Path))
	}
	sort.Strings(owPaths)
	sort.Strings(crPaths)
	t.Logf("overwritten: %v", owPaths)
	t.Logf("created: %v", crPaths)

	if len(owPaths) != 1 || owPaths[0] != "OptiScaler.ini" {
		t.Errorf("overwritten = %v, want [OptiScaler.ini]", owPaths)
	}
	for _, want := range []string{"dxgi.dll", "fakenvapi.dll", "fakenvapi.ini", "D3D12Core.dll"} {
		found := false
		for _, c := range crPaths {
			if c == want {
				found = true
			}
		}
		if !found {
			t.Errorf("created missing %q", want)
		}
	}
	if len(m.CreatedDirs) == 0 {
		t.Error("CreatedDirs empty; D3D12_Optiscaler should be recorded")
	}
}

func TestRollbackFromInProgress(t *testing.T) {
	root, bin, st := newGame(t)
	original := "ORIGINAL-DXGI"
	writeFile(t, filepath.Join(bin, "dxgi.dll"), original)

	calls := 0
	orig := copyFileFn
	copyFileFn = func(src, dst string) (string, error) {
		calls++
		if calls == 2 {
			return "", fmt.Errorf("injected copy failure on %s", dst)
		}
		return orig(src, dst)
	}
	defer func() { copyFileFn = orig }()

	_, err := Install(context.Background(), st, request(root, bin))
	if err == nil {
		t.Fatal("expected injected failure, got nil")
	}
	id := manifestID(t, bin)
	m, lerr := st.Load(id)
	if lerr != nil {
		t.Fatalf("manifest missing after failed install: %v", lerr)
	}
	if m.Status != domain.StatusFailed {
		t.Fatalf("status %q, want failed", m.Status)
	}
	t.Logf("failed as designed after %d copies: %v", calls, err)

	if err := Rollback(context.Background(), st, id); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(bin, "dxgi.dll")); string(data) != original {
		t.Error("original dxgi.dll not restored")
	}
	entries, _ := os.ReadDir(bin)
	for _, e := range entries {
		t.Logf("remaining in bin: %s", e.Name())
	}
	if _, err := os.Stat(filepath.Join(bin, "fakenvapi.dll")); !os.IsNotExist(err) {
		t.Error("created file fakenvapi.dll survived rollback")
	}
	if _, err := os.Stat(filepath.Join(bin, "D3D12_Optiscaler")); !os.IsNotExist(err) {
		t.Error("created dir D3D12_Optiscaler survived rollback")
	}
	m2, _ := st.Load(id)
	if m2.Status != domain.StatusRolledBack {
		t.Errorf("status %q, want rolled_back", m2.Status)
	}

	// Idempotent: second rollback is a no-op.
	if err := Rollback(context.Background(), st, id); err != nil {
		t.Errorf("second Rollback not idempotent: %v", err)
	}
}

func TestUninstallRefusesChangedFile(t *testing.T) {
	root, bin, st := newGame(t)
	if _, err := Install(context.Background(), st, request(root, bin)); err != nil {
		t.Fatalf("Install: %v", err)
	}

	victim := filepath.Join(bin, "fakenvapi.ini")
	writeFile(t, victim, "USER-EDITED")

	id := manifestID(t, bin)
	err := Uninstall(context.Background(), st, id)
	var rf *RefusedError
	if !errors.As(err, &rf) {
		t.Fatalf("expected RefusedError, got %v", err)
	}
	if len(rf.Paths) != 1 || rf.Paths[0] != victim {
		t.Errorf("refused paths %v, want [%s]", rf.Paths, victim)
	}
	if _, serr := os.Stat(victim); serr != nil {
		t.Error("refused file was deleted")
	}
	if _, serr := os.Stat(filepath.Join(bin, "dxgi.dll")); !os.IsNotExist(serr) {
		t.Error("matching created file dxgi.dll should be deleted")
	}
	m, _ := st.Load(id)
	if m == nil || m.Status != domain.StatusCommitted {
		t.Errorf("manifest should remain committed after refusal")
	}
	if len(m.Created) != 1 {
		t.Errorf("manifest should retain only the refused created entry, got %d", len(m.Created))
	}
	t.Logf("refused as designed: %v", rf)
}

func TestInstallUninstallRoundTrip(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "OptiScaler.ini"), "ORIGINAL-INI")
	before := snapshot(t, root)

	m, err := Install(context.Background(), st, request(root, bin))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	id := m.ID

	// Second install over a committed manifest is an error, not an update.
	if _, err := Install(context.Background(), st, request(root, bin)); err == nil {
		t.Error("second Install over committed manifest should fail")
	}

	if err := Uninstall(context.Background(), st, id); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	after := snapshot(t, root)
	if before != after {
		t.Errorf("game dir not restored byte-for-byte\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, err := st.Load(id); err == nil {
		t.Error("manifest should be deleted after clean uninstall")
	}
	if _, err := os.Stat(st.BackupDir(id)); !os.IsNotExist(err) {
		t.Error("backup dir should be deleted after clean uninstall")
	}
	t.Logf("round trip clean: %d files restored", strings.Count(before, "\n"))
}

// snapshot renders path:sha256 lines for every file under dir.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		lines = append(lines, rel+":"+sha(t, path))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
