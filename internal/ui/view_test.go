package ui

import "testing"

func TestToggleView(t *testing.T) {
	s := NewSession(Deps{})
	if got := s.Snapshot().Mode; got != ViewGrid {
		t.Fatalf("default mode %v, want grid", got)
	}
	s.ToggleView()
	if got := s.Snapshot().Mode; got != ViewList {
		t.Fatalf("after toggle %v, want list", got)
	}
	s.ToggleView()
	if got := s.Snapshot().Mode; got != ViewGrid {
		t.Fatalf("after second toggle %v, want grid", got)
	}
	t.Log("view mode toggles grid↔list")
}
