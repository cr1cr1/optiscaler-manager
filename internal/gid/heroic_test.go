package gid

import (
	"strings"
	"testing"
)

// heroicFixture is a minimal but realistic legendary/installed.json (Epic)
// as Heroic writes it: keyed by app name, snake_case fields.
const heroicEpicFixture = `{
  "Fortnite": {
    "app_name": "Fortnite",
    "title": "Fortnite",
    "version": "1.0",
    "install_path": "/games/fortnite",
    "executable": "FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe",
    "platform": "Windows",
    "is_dlc": false
  },
  "Sugar": {
    "app_name": "Sugar",
    "title": "A Game With No Platform Field",
    "install_path": "/games/sugar",
    "executable": "sugar.exe"
  }
}`

// heroicGOGFixture mirrors gog_store/installed.json: keyed by GOG id,
// camelCase appName, lowercase platform.
const heroicGOGFixture = `{
  "1207658930": {
    "appName": "1207658930",
    "title": "Hades",
    "install_path": "/games/hades",
    "executable": "Hades.exe",
    "platform": "windows"
  },
  "1423049311": {
    "appName": "1423049311",
    "title": "No Exe Recorded",
    "install_path": "/games/noexe",
    "platform": "windows"
  }
}`

func TestParseHeroicInstalledEpicShape(t *testing.T) {
	entries, err := ParseHeroicInstalled(strings.NewReader(heroicEpicFixture))
	if err != nil {
		t.Fatalf("ParseHeroicInstalled: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	fn := entries[0]
	if fn.AppName != "Fortnite" {
		t.Errorf("entries[0].AppName = %q, want Fortnite (sorted by app name)", fn.AppName)
	}
	if fn.Title != "Fortnite" {
		t.Errorf("Title = %q, want Fortnite", fn.Title)
	}
	if fn.InstallPath != "/games/fortnite" {
		t.Errorf("InstallPath = %q", fn.InstallPath)
	}
	if fn.Executable != "FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe" {
		t.Errorf("Executable = %q", fn.Executable)
	}
	if !fn.IsWindows() {
		t.Error("Fortnite platform=Windows must be a windows entry")
	}
	sugar := entries[1]
	if sugar.AppName != "Sugar" {
		t.Errorf("entries[1].AppName = %q, want Sugar", sugar.AppName)
	}
	if !sugar.IsWindows() {
		t.Error("empty platform (older legendary files) must count as windows")
	}
	t.Logf("epic entries: %+v", entries)
}

func TestParseHeroicInstalledGOGShape(t *testing.T) {
	entries, err := ParseHeroicInstalled(strings.NewReader(heroicGOGFixture))
	if err != nil {
		t.Fatalf("ParseHeroicInstalled: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	hades := entries[0]
	if hades.AppName != "1207658930" {
		t.Errorf("AppName = %q, want gog id 1207658930", hades.AppName)
	}
	if hades.Title != "Hades" {
		t.Errorf("Title = %q, want Hades", hades.Title)
	}
	if !hades.IsWindows() {
		t.Error("lowercase platform=windows must count as windows")
	}
	if entries[1].Executable != "" {
		t.Errorf("entries[1].Executable = %q, want empty", entries[1].Executable)
	}
	t.Logf("gog entries: %+v", entries)
}

func TestParseHeroicInstalledSkipsAndFallbacks(t *testing.T) {
	doc := `{
  "KeyOnly": {"title": "No AppName Field", "install_path": "/games/keyonly"},
  "MacGame": {"app_name": "MacGame", "title": "Mac", "install_path": "/games/mac", "platform": "Mac"},
  "NoPath": {"app_name": "NoPath", "title": "No install path"},
  "NoTitle": {"app_name": "NoTitle", "install_path": "/games/notitle"}
}`
	entries, err := ParseHeroicInstalled(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseHeroicInstalled: %v", err)
	}
	// MacGame keeps parsing (IsWindows is the probe's filter); NoPath is
	// dropped — an entry without an install dir is unusable.
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (NoPath dropped): %+v", len(entries), entries)
	}
	byName := map[string]HeroicEntry{}
	for _, e := range entries {
		byName[e.AppName] = e
	}
	if e := byName["KeyOnly"]; e.AppName != "KeyOnly" {
		t.Errorf("map key must backfill a missing app_name: %+v", e)
	}
	if e := byName["MacGame"]; e.IsWindows() {
		t.Error("platform=Mac must not count as windows")
	}
	if e := byName["NoTitle"]; e.Title != "NoTitle" {
		t.Errorf("empty title must fall back to app name, got %q", e.Title)
	}
	t.Logf("fallback entries: %+v", entries)
}

func TestParseHeroicInstalledRejectsBrokenJSON(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":    `{"unterminated`,
		"not object":  `["array"]`,
		"wrong value": `{"X": 42}`,
	} {
		if _, err := ParseHeroicInstalled(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
