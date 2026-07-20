package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnlineLookups_DefaultsTrueWhenMissing(t *testing.T) {
	// A settings.json written before the key existed must load with
	// online lookups enabled (backward compatible: missing key -> true).
	root := t.TempDir()
	legacy := `{"default_version":"latest","launch_template":"\"{exe}\" {args}"}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.OnlineLookups {
		t.Error("OnlineLookups = false for a legacy file without the key, want true (default)")
	}

	// Defaults() (no file at all) enables lookups too.
	d := Defaults()
	if !d.OnlineLookups {
		t.Error("Defaults().OnlineLookups = false, want true")
	}
	t.Log("legacy settings.json without the key -> OnlineLookups true")
}

func TestOnlineLookups_ExplicitFalseSurvives(t *testing.T) {
	root := t.TempDir()
	legacy := `{"default_version":"latest","online_lookups":false}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.OnlineLookups {
		t.Error("OnlineLookups = true for an explicit false in the file, want false")
	}
	// A Load+Save round-trip must not flip it back on.
	if err := Save(root, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.OnlineLookups {
		t.Error("OnlineLookups flipped to true after a Load+Save round-trip, want false")
	}

	// Explicit true survives too.
	if err := Save(root, Settings{OnlineLookups: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(root)
	if !got.OnlineLookups {
		t.Error("explicit true did not survive the round-trip")
	}
	t.Log("explicit false and true both survive Load+Save round-trips")
}
