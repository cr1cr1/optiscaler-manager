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
