package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Issue 026: installing extracts only the files the game needs. Markdown
// documentation and the distribution's own install/remove scripts never
// reach the game dir, and a directory that would hold only filtered files
// is never created (no empty dirs).
func TestInstallFiltersMarkdownScriptsAndEmptyDirs(t *testing.T) {
	root, bin, st := newGame(t)
	bundle := filepath.Join(t.TempDir(), "cluttered.zip")
	writeZip(t, bundle, map[string]string{
		"OptiScaler.dll":        "INJECTOR",
		"OptiScaler.ini":        "INI",
		"fakenvapi.dll":         "FNV",
		"README.MD":             "readme",       // case-insensitive
		"CHANGELOG.markdown":    "changelog",    // .markdown counts too
		"docs/guide.md":         "only-md-here", // docs/ would be empty
		"setup_windows.bat":     "setup",
		"Remove OptiScaler.bat": "remove",
		"deploy.cmd":            "cmd",
		"install.ps1":           "ps1",
		"setup.sh":              "sh",
	})

	req := request(root, bin)
	req.ArchivePath = bundle
	m, err := Install(context.Background(), st, req)
	if err != nil {
		t.Fatalf("Install(cluttered): %v", err)
	}

	// The real payload is installed.
	for _, rel := range []string{"dxgi.dll", "fakenvapi.dll", "OptiScaler.ini"} {
		if _, err := os.Stat(filepath.Join(bin, rel)); err != nil {
			t.Errorf("payload %s missing after install: %v", rel, err)
		}
	}

	// No clutter reaches the game dir or the manifest.
	clutter := []string{
		"README.MD", "CHANGELOG.markdown", "setup_windows.bat",
		"Remove OptiScaler.bat", "deploy.cmd", "install.ps1", "setup.sh",
		filepath.Join("docs", "guide.md"),
	}
	for _, rel := range clutter {
		if _, err := os.Stat(filepath.Join(bin, rel)); !os.IsNotExist(err) {
			t.Errorf("filtered clutter %s reached the game dir", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(bin, "docs")); !os.IsNotExist(err) {
		t.Error("empty docs/ dir created for filtered markdown")
	}
	for _, c := range m.Created {
		for _, rel := range clutter {
			if c.Path == filepath.Join(bin, rel) {
				t.Errorf("filtered clutter %s tracked in the manifest", rel)
			}
		}
	}
}

// The plan-level filter is extension-based and case-insensitive; directory
// members were already skipped (issue 026 piggybacks on that for empty
// dirs).
func TestBuildPlanFiltersMarkdownAndScripts(t *testing.T) {
	plan, err := buildPlan([]string{
		"OptiScaler.dll",
		"ReadMe.Md",
		"guide.MARKDOWN",
		"Setup.BAT",
		"remove.cmd",
		"INSTALL.PS1",
		"run.sh",
		"docs/",
	})
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if len(plan) != 1 || plan[0].dstRel != injectionDLL {
		t.Errorf("plan = %v, want only the renamed injector", plan)
	}
}
