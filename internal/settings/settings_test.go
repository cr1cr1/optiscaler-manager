package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.DefaultVersion != "latest" {
		t.Errorf("DefaultVersion %q, want latest", s.DefaultVersion)
	}
	if len(s.ExtraDirs) != 0 {
		t.Errorf("ExtraDirs %v, want empty", s.ExtraDirs)
	}
}

func TestCardSizeOrDefault(t *testing.T) {
	for _, s := range []CardSize{CardSizeSmall, CardSizeMedium, CardSizeLarge} {
		if got := s.OrDefault(); got != s {
			t.Errorf("CardSize(%q).OrDefault() = %q, want itself", s, got)
		}
	}
	for _, s := range []CardSize{"", "bogus", "SMALL"} {
		if got := s.OrDefault(); got != CardSizeMedium {
			t.Errorf("CardSize(%q).OrDefault() = %q, want medium", s, got)
		}
	}
	t.Log("card size presets normalize to medium")
}

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := Settings{DefaultVersion: "v0.9.4", ExtraDirs: []string{"/games/custom"}}
	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DefaultVersion != want.DefaultVersion {
		t.Errorf("DefaultVersion %q, want %q", got.DefaultVersion, want.DefaultVersion)
	}
	if len(got.ExtraDirs) != 1 || got.ExtraDirs[0] != "/games/custom" {
		t.Errorf("ExtraDirs %v", got.ExtraDirs)
	}

	// Empty version normalizes back to latest.
	if err := Save(root, Settings{DefaultVersion: ""}); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(root)
	if got.DefaultVersion != "latest" {
		t.Errorf("empty version normalized to %q, want latest", got.DefaultVersion)
	}
	t.Log("settings round-trip + normalization ok")
}

func TestLaunchTemplatePersists(t *testing.T) {
	// Defaults carry the plain exe+args template.
	d := Defaults()
	if d.LaunchTemplate != `"{exe}" {args}` {
		t.Fatalf("Defaults().LaunchTemplate %q, want %q", d.LaunchTemplate, `"{exe}" {args}`)
	}

	// A custom template survives the save/load round-trip.
	root := t.TempDir()
	custom := `umu-run "{exe}" {args}`
	if err := Save(root, Settings{LaunchTemplate: custom}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LaunchTemplate != custom {
		t.Errorf("persisted LaunchTemplate %q, want %q", got.LaunchTemplate, custom)
	}

	// Empty in JSON normalizes back to the default at load.
	if err := Save(root, Settings{LaunchTemplate: ""}); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(root)
	if got.LaunchTemplate != `"{exe}" {args}` {
		t.Errorf("empty template normalized to %q, want default", got.LaunchTemplate)
	}
	t.Log("launch template: default, custom round-trip, empty normalization ok")
}

func TestSaveEmptyRootIsNoOp(t *testing.T) {
	if err := Save("", Defaults()); err != nil {
		t.Fatalf("Save with empty root must be a no-op, got %v", err)
	}
}

// Title overrides persist: a user's pinned title for a canonical install
// dir survives a Save/Load roundtrip; absent key yields nil, not empty map.
func TestSettings_TitleOverridesRoundtrip(t *testing.T) {
	root := t.TempDir()
	s := Defaults()
	if s.TitleOverrides != nil {
		t.Fatalf("Defaults().TitleOverrides = %v, want nil (no key written)", s.TitleOverrides)
	}
	s.TitleOverrides = map[string]string{"/games/Prey": "Prey (2017)"}
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.TitleOverrides["/games/Prey"] != "Prey (2017)" {
		t.Errorf("TitleOverrides = %v, want the pinned title", got.TitleOverrides)
	}
}

// umu-launcher integration defaults: opt-in (off), no Proton path pin
// (let umu fall back to UMU-Latest).
func TestSettings_UmuDefaults(t *testing.T) {
	s := Defaults()
	if s.UmuEnabled {
		t.Errorf("Defaults().UmuEnabled = true, want false (opt-in)")
	}
	if s.UmuProtonPath != "" {
		t.Errorf("Defaults().UmuProtonPath = %q, want empty", s.UmuProtonPath)
	}
}

// Umu fields survive a save/load round-trip with both the enabled flag
// and a pinned Proton path intact.
func TestSettings_UmuRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := Settings{
		DefaultVersion: "latest",
		LaunchTemplate: DefaultLaunchTemplate,
		OnlineLookups:  true,
		UmuEnabled:     true,
		UmuProtonPath:  "/runners/GE-Proton9-3",
	}
	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UmuEnabled != want.UmuEnabled {
		t.Errorf("UmuEnabled = %v, want %v", got.UmuEnabled, want.UmuEnabled)
	}
	if got.UmuProtonPath != want.UmuProtonPath {
		t.Errorf("UmuProtonPath = %q, want %q", got.UmuProtonPath, want.UmuProtonPath)
	}
}

// A legacy settings.json written before umu-launcher support existed
// must load with the safe defaults: UmuEnabled=false, UmuProtonPath="".
// This protects users upgrading from older releases.
func TestSettings_LegacyJSONWithoutUmuKeys(t *testing.T) {
	root := t.TempDir()
	legacy := `{"default_version":"v0.10.0","launch_template":"\"{exe}\" {args}","extra_dirs":["/games"]}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	if got.UmuEnabled {
		t.Errorf("UmuEnabled = true on legacy file, want false (opt-in default)")
	}
	if got.UmuProtonPath != "" {
		t.Errorf("UmuProtonPath = %q on legacy file, want empty", got.UmuProtonPath)
	}
}
