package discovery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Scratch real-tree probe (deleted after the v0.7.2 verification): scans
// the user's actual library roots and asserts the reported defects are
// gone. Not a repo test — evidence gathering only.
func TestScratchRealTree(t *testing.T) {
	start := time.Now()
	var all []string
	rows := map[string]string{}
	for _, root := range []string{"/mnt/linux2/Games", "/mnt/linux3/Games"} {
		games, err := ScanRecursive(context.Background(), root)
		if err != nil {
			t.Fatalf("ScanRecursive(%s): %v", root, err)
		}
		for _, g := range games {
			rows[canonicalPath(g.InstallDir)] = g.Name
			all = append(all, g.Name+"\t"+g.InstallDir)
		}
	}
	t.Logf("scanned in %s, %d rows", time.Since(start).Round(time.Second), len(all))
	for _, r := range all {
		t.Logf("row: %s", r)
	}

	banned := []string{"steam", "steamapps", "compatdata", "shadercache", "downloading",
		"program files", "windows", "system32", "syswow64", "drive_c", "files", "bin64",
		"x64_dx12", "retail", "redmod", "crs", "4.8", "4.6", "8.09.04", "2.0.7.0",
		"_redist", "__installer", "dotnet", "physx", "openal", "proton - experimental",
		"proton hotfix", "steamlinuxruntime", "steamlinuxruntime_soldier", "steamlinuxruntime_sniper"}
	for dir, name := range rows {
		base := strings.ToLower(filepath.Base(dir))
		for _, b := range banned {
			if base == b {
				t.Errorf("junk row: name=%q dir=%q", name, dir)
			}
		}
		if strings.Contains(strings.ToLower(dir), "proton") || strings.Contains(strings.ToLower(dir), "steamlinuxruntime") {
			t.Errorf("proton/SLR row: name=%q dir=%q", name, dir)
		}
	}

	expect := map[string]string{
		"/mnt/linux3/Games/The Witcher 3 - Wild Hunt/bin/x64_dx12":  "",
		"/mnt/linux2/Games/Crysis Remastered/Bin64":                 "",
		"/mnt/linux3/Games/007 First Light/Retail":                  "",
		"/mnt/linux3/Games/Cyberpunk 2077/tools/redmod":             "",
		"/mnt/linux3/Games/Days Gone Broken Road/Engine/Binaries/ThirdParty/CRS": "",
	}
	for junk := range expect {
		if _, ok := rows[junk]; ok {
			t.Errorf("wrong-level row still present: %q", junk)
		}
	}
	for _, want := range []string{
		"/mnt/linux3/Games/The Witcher 3 - Wild Hunt",
		"/mnt/linux2/Games/Crysis Remastered",
		"/mnt/linux3/Games/007 First Light",
		"/mnt/linux3/Games/Cyberpunk 2077",
	} {
		if _, ok := rows[want]; !ok {
			t.Errorf("expected game-root row missing: %q", want)
		}
	}
	if name, ok := rows["/mnt/linux3/Games/Dead Space Remake"]; !ok {
		t.Error("Dead Space Remake row missing")
	} else if !strings.Contains(strings.ToLower(name), "dead space") {
		t.Errorf("Dead Space title = %q, want PE metadata title", name)
	} else {
		t.Logf("Dead Space title from PE: %q", name)
	}
}
