package ui

import (
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// TestParseEnv: the launch-env edit format is whitespace-separated
// KEY=VALUE pairs (values in practice are 0/1/lists); a token without
// '=' is an error naming it (issue 067).
func TestParseEnv(t *testing.T) {
	m, err := ParseEnv("DXVK_HUD=1 PROTON_ENABLE_NVAPI=1")
	if err != nil {
		t.Fatalf("ParseEnv: %v", err)
	}
	if m["DXVK_HUD"] != "1" || m["PROTON_ENABLE_NVAPI"] != "1" || len(m) != 2 {
		t.Errorf("ParseEnv = %v, want the two pairs", m)
	}

	m, err = ParseEnv("  ")
	if err != nil || len(m) != 0 {
		t.Errorf("ParseEnv(blank) = %v, %v; want empty, nil", m, err)
	}

	if _, err = ParseEnv("DXVK_HUD"); err == nil {
		t.Error("token without =: want error, got nil")
	}
	if _, err = ParseEnv("=1"); err == nil {
		t.Error("empty key: want error, got nil")
	}
}

// TestFormatEnvRoundTrip: the canonical form sorts keys so the input
// field is stable; ParseEnv(FormatEnv(m)) is m (issue 067).
func TestFormatEnvRoundTrip(t *testing.T) {
	in := map[string]string{"PROTON_ENABLE_NVAPI": "1", "DXVK_HUD": "fps", "WINEDLLOVERRIDES": "d3d11=n"}
	got := FormatEnv(in)
	want := "DXVK_HUD=fps PROTON_ENABLE_NVAPI=1 WINEDLLOVERRIDES=d3d11=n"
	if got != want {
		t.Errorf("FormatEnv = %q, want %q", got, want)
	}
	back, err := ParseEnv(got)
	if err != nil || len(back) != len(in) {
		t.Fatalf("round trip: %v, %v", back, err)
	}
	for k, v := range in {
		if back[k] != v {
			t.Errorf("round trip[%s] = %q, want %q", k, back[k], v)
		}
	}
}

// TestSessionUmuEnvOverride: per-game env pins persist and clear;
// UmuEnvFor returns a copy (mutation-safe); unknown games refuse
// (issue 067).
func TestSessionUmuEnvOverride(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.Scan(t.Context())
	waitScanDone(t, e.sess)
	dir := e.gameRoot

	if got := e.sess.UmuEnvFor(dir); len(got) != 0 {
		t.Errorf("UmuEnvFor(no pin) = %v, want empty", got)
	}

	e.sess.SetUmuEnvOverride(dir, map[string]string{"DXVK_HUD": "1"})
	got := e.sess.UmuEnvFor(dir)
	if got["DXVK_HUD"] != "1" {
		t.Errorf("UmuEnvFor = %v, want DXVK_HUD=1", got)
	}
	got["DXVK_HUD"] = "mutated"
	if e.sess.UmuEnvFor(dir)["DXVK_HUD"] != "1" {
		t.Error("UmuEnvFor exposes the live map — must return a copy")
	}

	s, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.EnvOverrides[canonicalDir(dir)]["DXVK_HUD"] != "1" {
		t.Errorf("persisted EnvOverrides = %v, want the pin", s.EnvOverrides)
	}

	e.sess.SetUmuEnvOverride(dir, nil)
	if len(e.sess.Settings().EnvOverrides) != 0 {
		t.Errorf("after clear EnvOverrides = %v, want empty", e.sess.Settings().EnvOverrides)
	}

	e.sess.SetUmuEnvOverride("/no/such/game", map[string]string{"A": "1"})
	if len(e.sess.Settings().EnvOverrides) != 0 {
		t.Errorf("override persisted for unknown game: %v", e.sess.Settings().EnvOverrides)
	}
}
