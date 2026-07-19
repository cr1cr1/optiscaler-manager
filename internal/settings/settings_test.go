package settings

import "testing"

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
