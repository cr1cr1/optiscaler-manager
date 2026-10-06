package installer

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeZip builds a synthetic .zip bundle at path. Forks publish zips (the
// archive package dispatches on extension), and their layouts differ from
// upstream's flat 7z — issue 10's fixtures are built at runtime so the
// layout under test is visible in the test.
func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic archive order
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// dlssnrShaped mirrors the real OptiScaler-NR (DLSSNR) zip layout: the
// injector and its ini at the archive root, the support DLLs under an
// OptiScaler/ subdir (paths the mod expects relative to the game dir),
// docs at the root — and NO fakenvapi at all.
func dlssnrShaped() map[string]string {
	return map[string]string{
		"OptiScaler.dll":                            "FORK-INJECTOR",
		"OptiScaler.ini":                            "FORK-INI",
		"OptiScaler/libxess.dll":                    "FORK-XESS",
		"OptiScaler/libxell.dll":                    "FORK-XELL",
		"OptiScaler/amd_fidelityfx_dx12.dll":        "FORK-FFX",
		"OptiScaler/D3D12_OptiScaler/D3D12Core.dll": "FORK-D3D12",
		"README.md":                                 "FORK-README",
		"docs/NR-VULKAN.md":                         "FORK-DOC-VULKAN",
	}
}

func zipNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The required set is derived from the distribution's own archive listing:
// the only universal file is the injector dll — forks like DLSSNR ship no
// fakenvapi, and their support files may live under a subdir, which the
// plan must preserve verbatim (never stripped).
func TestBuildPlanRequiresOnlyTheInjector(t *testing.T) {
	plan, err := buildPlan(zipNames(dlssnrShaped()))
	if err != nil {
		t.Fatalf("buildPlan(dlssnr-shaped, no fakenvapi): %v", err)
	}
	byDst := map[string]string{}
	for _, fp := range plan {
		byDst[fp.dstRel] = fp.srcRel
	}
	if byDst["dxgi.dll"] != "OptiScaler.dll" {
		t.Errorf("injection rename missing: plan=%v", plan)
	}
	nested := filepath.Join("OptiScaler", "libxess.dll")
	if byDst[nested] != nested {
		t.Errorf("nested subdir path not preserved verbatim: plan=%v", plan)
	}
	deep := filepath.Join("OptiScaler", "D3D12_OptiScaler", "D3D12Core.dll")
	if byDst[deep] != deep {
		t.Errorf("deeply nested path not preserved verbatim: plan=%v", plan)
	}
	if byDst[filepath.Join("docs", "NR-VULKAN.md")] == "" {
		t.Errorf("root-adjacent docs missing from plan: %v", plan)
	}

	// The injector is the one universal requirement.
	if _, err := buildPlan([]string{"fakenvapi.dll", "fakenvapi.ini", "README.md"}); err == nil {
		t.Error("buildPlan without OptiScaler.dll succeeded, want rejection")
	}
}

// A DLSSNR-shaped bundle installs end to end: injector renamed at the
// injection dir root, subdir support files verbatim, docs verbatim, and
// the bundle ini replaced by the curated defaults.
func TestInstallDLSSNRShapedZip(t *testing.T) {
	root, bin, st := newGame(t)
	bundle := filepath.Join(t.TempDir(), "OptiScaler-NR-test.zip")
	writeZip(t, bundle, dlssnrShaped())

	req := request(root, bin)
	req.ArchivePath = bundle
	req.Fork = "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass"
	m, err := Install(context.Background(), st, req)
	if err != nil {
		t.Fatalf("Install(dlssnr-shaped): %v", err)
	}

	for rel, want := range map[string]string{
		"dxgi.dll": "FORK-INJECTOR",
		filepath.Join("OptiScaler", "libxess.dll"):                       "FORK-XESS",
		filepath.Join("OptiScaler", "D3D12_OptiScaler", "D3D12Core.dll"): "FORK-D3D12",
		filepath.Join("docs", "NR-VULKAN.md"):                            "FORK-DOC-VULKAN",
		"README.md":                                                      "FORK-README",
	} {
		data, err := os.ReadFile(filepath.Join(bin, rel))
		if err != nil {
			t.Errorf("installed %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("installed %s = %q, want %q", rel, data, want)
		}
	}
	// The bundle ini is replaced by the curated defaults.
	if data, err := os.ReadFile(filepath.Join(bin, "OptiScaler.ini")); err != nil || string(data) == "FORK-INI" {
		t.Errorf("curated ini not in place: %q (err %v)", data, err)
	}
	// Every planned file is manifest-tracked (nested paths included) —
	// the manifest IS the old distribution's file set on a later switch.
	tracked := map[string]bool{}
	for _, c := range m.Created {
		tracked[c.Path] = true
	}
	if !tracked[filepath.Join(bin, "OptiScaler", "libxess.dll")] {
		t.Error("manifest does not track the nested OptiScaler/libxess.dll")
	}
}

// A bundle without OptiScaler.ini still gets the curated defaults, tracked
// as a created file (the uninstall/switch file set must include it).
func TestInstallBundleWithoutINIDropsCuratedDefaults(t *testing.T) {
	root, bin, st := newGame(t)
	files := dlssnrShaped()
	delete(files, "OptiScaler.ini")
	bundle := filepath.Join(t.TempDir(), "noini.zip")
	writeZip(t, bundle, files)

	req := request(root, bin)
	req.ArchivePath = bundle
	m, err := Install(context.Background(), st, req)
	if err != nil {
		t.Fatalf("Install(no ini in bundle): %v", err)
	}
	iniPath := filepath.Join(bin, "OptiScaler.ini")
	if _, err := os.Stat(iniPath); err != nil {
		t.Fatalf("curated ini missing after install: %v", err)
	}
	for _, c := range m.Created {
		if c.Path == iniPath && c.SHA256 != "" {
			return
		}
	}
	t.Error("curated ini not tracked as a created manifest entry")
}

// Uninstall with a relocate dir (fork switch) MOVES the old distribution's
// file set — nested paths preserved — instead of deleting it, and prunes
// the now-empty created dirs.
func TestUninstallRelocateMovesMatchedFiles(t *testing.T) {
	root, bin, st := newGame(t)
	bundle := filepath.Join(t.TempDir(), "fork.zip")
	writeZip(t, bundle, dlssnrShaped())
	req := request(root, bin)
	req.ArchivePath = bundle
	if _, err := Install(context.Background(), st, req); err != nil {
		t.Fatalf("Install: %v", err)
	}

	dated := filepath.Join(bin, "OptiScaler-DLSSNR-PreSR-Multipass.261007")
	id := manifestID(t, bin)
	if err := UninstallWithOptions(context.Background(), st, id, UninstallOptions{RelocateDir: dated}); err != nil {
		t.Fatalf("UninstallWithOptions(relocate): %v", err)
	}

	for rel, want := range map[string]string{
		"dxgi.dll": "FORK-INJECTOR",
		filepath.Join("OptiScaler", "libxess.dll"): "FORK-XESS",
		"README.md": "FORK-README",
	} {
		data, err := os.ReadFile(filepath.Join(dated, rel))
		if err != nil {
			t.Errorf("relocated %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("relocated %s = %q, want %q", rel, data, want)
		}
		if _, err := os.Stat(filepath.Join(bin, rel)); !os.IsNotExist(err) {
			t.Errorf("%s still in the game dir after relocation", rel)
		}
	}
	// The curated ini is part of the installed set: relocated too.
	if _, err := os.Stat(filepath.Join(dated, "OptiScaler.ini")); err != nil {
		t.Errorf("curated ini not relocated: %v", err)
	}
	// Now-empty created dirs are pruned from the game dir.
	if _, err := os.Stat(filepath.Join(bin, "OptiScaler")); !os.IsNotExist(err) {
		t.Error("empty OptiScaler/ subdir left behind after relocation")
	}
	// The manifest is gone: the game is clean and installable.
	if _, err := st.Load(id); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("manifest after relocation = %v, want deleted", err)
	}
}

// Relocation restores SHA-verified pre-install originals for overwritten
// files after moving the old fork's bytes out, and still refuses
// foreign-modified files (never moved, never deleted).
func TestUninstallRelocateRestoresOriginalAndRefusesForeign(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "dxgi.dll"), "ORIGINAL-DXGI")
	bundle := filepath.Join(t.TempDir(), "fork.zip")
	writeZip(t, bundle, dlssnrShaped())
	req := request(root, bin)
	req.ArchivePath = bundle
	if _, err := Install(context.Background(), st, req); err != nil {
		t.Fatalf("Install: %v", err)
	}

	dated := filepath.Join(bin, "old.261007")
	id := manifestID(t, bin)
	if err := UninstallWithOptions(context.Background(), st, id, UninstallOptions{RelocateDir: dated}); err != nil {
		t.Fatalf("UninstallWithOptions(relocate over overwrite): %v", err)
	}
	// The old fork's injector bytes moved out; the original is back.
	if data, _ := os.ReadFile(filepath.Join(dated, "dxgi.dll")); string(data) != "FORK-INJECTOR" {
		t.Errorf("relocated dxgi.dll = %q, want the old fork's bytes", data)
	}
	if data, _ := os.ReadFile(filepath.Join(bin, "dxgi.dll")); string(data) != "ORIGINAL-DXGI" {
		t.Errorf("restored dxgi.dll = %q, want the pre-install original", data)
	}

	// A foreign-modified file is refused, never moved.
	root2, bin2, st2 := newGame(t)
	bundle2 := filepath.Join(t.TempDir(), "fork2.zip")
	writeZip(t, bundle2, dlssnrShaped())
	req2 := request(root2, bin2)
	req2.ArchivePath = bundle2
	if _, err := Install(context.Background(), st2, req2); err != nil {
		t.Fatalf("Install(2): %v", err)
	}
	writeFile(t, filepath.Join(bin2, "README.md"), "USER-EDITED")
	dated2 := filepath.Join(bin2, "old.261007")
	err := UninstallWithOptions(context.Background(), st2, manifestID(t, bin2), UninstallOptions{RelocateDir: dated2})
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("relocate over foreign-modified file = %v, want RefusedError", err)
	}
	if data, _ := os.ReadFile(filepath.Join(bin2, "README.md")); string(data) != "USER-EDITED" {
		t.Error("foreign-modified file was moved or deleted")
	}
	if _, err := os.Stat(filepath.Join(dated2, "README.md")); !os.IsNotExist(err) {
		t.Error("foreign-modified file was relocated")
	}
}
