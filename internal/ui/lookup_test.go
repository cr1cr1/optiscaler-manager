package ui

import (
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// TestOnlineLookups_Persists: the session toggle updates the in-memory
// settings and persists them like SetDefaultVersion.
func TestOnlineLookups_Persists(t *testing.T) {
	root := t.TempDir()
	sess := NewSession(Deps{SettingsRoot: root, Settings: settings.Defaults()})

	if !sess.Settings().OnlineLookups {
		t.Fatal("OnlineLookups = false on a fresh session, want true (default)")
	}
	sess.SetOnlineLookups(false)
	if sess.Settings().OnlineLookups {
		t.Fatal("OnlineLookups still true in memory after SetOnlineLookups(false)")
	}
	got, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings.Load: %v", err)
	}
	if got.OnlineLookups {
		t.Error("persisted OnlineLookups = true after SetOnlineLookups(false), want false")
	}

	sess.SetOnlineLookups(true)
	got, err = settings.Load(root)
	if err != nil {
		t.Fatalf("settings.Load: %v", err)
	}
	if !got.OnlineLookups {
		t.Error("persisted OnlineLookups = false after SetOnlineLookups(true), want true")
	}
	t.Log("toggle persists false and true across settings.Load")
}
