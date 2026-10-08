package ui

import (
	"slices"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// TestUmuPickerRowsComposition: the shared picker rows are Auto first,
// then the detected runners in scan order, plus a Custom row when the
// pinned path matches no detected runner.
func TestUmuPickerRowsComposition(t *testing.T) {
	runners := []umu.Runner{
		{Name: "GE-Proton10-1", Path: "/runners/GE-Proton10-1", Source: "heroic", Version: "10.1"},
		{Name: "GE-Proton9-3", Path: "/runners/GE-Proton9-3", Source: "steam", Version: "9.3"},
	}
	labels := func(rows []UmuPickerOption) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.Label
		}
		return out
	}

	auto := UmuPickerRows("", runners)
	if got, want := labels(auto), []string{"Auto (newest detected)", "GE-Proton10-1 (heroic)", "GE-Proton9-3 (steam)"}; !slices.Equal(got, want) {
		t.Errorf("rows(empty) = %v, want %v", got, want)
	}
	if auto[0].Value != "" {
		t.Errorf("Auto row value = %q, want empty", auto[0].Value)
	}

	known := UmuPickerRows("/runners/GE-Proton9-3", runners)
	if len(known) != 3 {
		t.Errorf("rows(known pin) = %d rows, want 3 (no Custom row)", len(known))
	}

	custom := UmuPickerRows("/elsewhere/proton", runners)
	last := custom[len(custom)-1]
	if last.Value != "/elsewhere/proton" || last.Label != "Custom: /elsewhere/proton" {
		t.Errorf("Custom row = %+v, want value/label for /elsewhere/proton", last)
	}

	if got := UmuPickerLabel("", runners); got != "Auto (newest detected)" {
		t.Errorf("label(empty) = %q, want Auto (newest detected)", got)
	}
	if got := UmuPickerLabel("/runners/GE-Proton10-1", runners); got != "GE-Proton10-1 (heroic)" {
		t.Errorf("label(known) = %q, want GE-Proton10-1 (heroic)", got)
	}
	if got := UmuPickerLabel("/elsewhere/proton", runners); got != "Custom: /elsewhere/proton" {
		t.Errorf("label(unknown) = %q, want Custom: /elsewhere/proton", got)
	}
}

// TestSessionUmuRunners_UsesInjectedProvider: the settings frontends
// read the picker entries from the injected deps provider verbatim.
func TestSessionUmuRunners_UsesInjectedProvider(t *testing.T) {
	e := newTestEnv(t)
	want := []umu.Runner{
		{Name: "GE-Proton10-1", Path: "/runners/GE-Proton10-1", Source: "heroic", Version: "10.1"},
		{Name: "GE-Proton9-3", Path: "/runners/GE-Proton9-3", Source: "steam", Version: "9.3"},
	}
	e.sess.deps.UmuRunners = func() []umu.Runner { return want }

	got := e.sess.UmuRunners()
	if len(got) != len(want) {
		t.Fatalf("len(UmuRunners()) = %d, want %d", len(got), len(want))
	}
	for i, r := range want {
		if got[i] != r {
			t.Errorf("UmuRunners()[%d] = %+v, want %+v", i, got[i], r)
		}
	}
}

// TestSessionUmuRunners_NilProviderFallsBackToScan: without an injected
// provider the production scan runs; on an empty HOME it finds nothing.
func TestSessionUmuRunners_NilProviderFallsBackToScan(t *testing.T) {
	e := newTestEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if got := e.sess.UmuRunners(); len(got) != 0 {
		t.Errorf("UmuRunners() = %v, want none on an empty HOME", got)
	}
}
