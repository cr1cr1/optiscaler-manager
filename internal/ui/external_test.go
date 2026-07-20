package ui

import (
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestCanOpenINI: the OptiScaler.ini affordance opens for every install that
// has one on disk — manager-committed AND external (detected, unmanaged) —
// and stays closed for every state without a usable install.
func TestCanOpenINI(t *testing.T) {
	tests := []struct {
		name   string
		status domain.Status
		want   bool
	}{
		{"committed install", domain.StatusCommitted, true},
		{"external install", domain.StatusExternal, true},
		{"failed install", domain.StatusFailed, false},
		{"never installed", domain.Status(""), false},
		{"install in progress", domain.StatusInProgress, false},
		{"rolled back", domain.StatusRolledBack, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := GameRow{Status: tt.status}
			if got := row.CanOpenINI(); got != tt.want {
				t.Errorf("GameRow{Status: %q}.CanOpenINI() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
