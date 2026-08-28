package ui

import (
	"context"
	"slices"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func TestRowPlatformFromStore(t *testing.T) {
	sess := NewSession(Deps{})
	stores := []domain.Store{domain.StoreSteam, domain.StoreEpic, domain.StoreGOG, domain.StoreManual}
	for _, st := range stores {
		row := sess.toRow(context.Background(), app.LibraryEntry{Game: domain.Game{
			Name:    "X",
			Store:   st,
			AppName: "AppNameX",
			ExePath: "/games/x/x.exe",
		}})
		t.Logf("store=%v → platform=%q row.Store=%v", st, row.Platform, row.Store)
		if row.Platform != st.String() {
			t.Errorf("store %v: Platform = %q, want %q", st, row.Platform, st.String())
		}
		if row.Store != st {
			t.Errorf("store %v: row.Store = %v, want raw store carried through", st, row.Store)
		}
		if row.AppName != "AppNameX" || row.ExePath != "/games/x/x.exe" {
			t.Errorf("store %v: AppName/ExePath not carried: %+v", st, row)
		}
	}
}

func TestRowHasInstall(t *testing.T) {
	cases := []struct {
		status domain.Status
		want   bool
	}{
		{domain.StatusCommitted, true},
		{domain.StatusExternal, true},
		{"", false},
		{domain.StatusFailed, false},
		{domain.StatusInProgress, false},
		{domain.StatusRolledBack, false},
	}
	for _, c := range cases {
		row := GameRow{Status: c.status}
		if got := row.HasInstall(); got != c.want {
			t.Errorf("HasInstall(%q) = %v, want %v", c.status, got, c.want)
		}
		// CanOpenINI shares the predicate: only an install has a usable ini.
		if got := row.CanOpenINI(); got != c.want {
			t.Errorf("CanOpenINI(%q) = %v, want %v (shares HasInstall)", c.status, got, c.want)
		}
	}
	t.Log("install predicate covers committed/external only")
}

func TestRowDisableToggleLabel(t *testing.T) {
	if label, ok := (GameRow{Status: domain.StatusCommitted}).DisableToggleLabel(); !ok || label != "Disable OptiScaler" {
		t.Errorf("active install: label %q ok %v, want \"Disable OptiScaler\"", label, ok)
	}
	if label, ok := (GameRow{Status: domain.StatusExternal, Disabled: true}).DisableToggleLabel(); !ok || label != "Enable OptiScaler" {
		t.Errorf("disabled external: label %q ok %v, want \"Enable OptiScaler\"", label, ok)
	}
	if _, ok := (GameRow{}).DisableToggleLabel(); ok {
		t.Error("plain game: toggle offered, want hidden (nothing to rename)")
	}
	t.Log("disable toggle captions derive from the row")
}

func TestInterruptedRows(t *testing.T) {
	rows := []GameRow{
		{Title: "Fine", Status: domain.StatusCommitted},
		{Title: "Died", Status: domain.StatusFailed, Actionable: true},
		{Title: "Halfway", Status: domain.StatusInProgress, Actionable: true},
		{Title: "Plain"},
	}
	got := InterruptedRows(rows)
	if len(got) != 2 || got[0].Title != "Died" || got[1].Title != "Halfway" {
		t.Fatalf("InterruptedRows = %v, want [Died Halfway]", got)
	}
	if len(InterruptedRows(nil)) != 0 {
		t.Error("InterruptedRows(nil) non-empty")
	}
	if msg := InterruptedMessage(got); msg != "2 interrupted installs: rollback to restore or install to retry" {
		t.Errorf("InterruptedMessage(2) = %q", msg)
	}
	if msg := InterruptedMessage(got[:1]); msg != "interrupted install: Died, rollback to restore or install to retry" {
		t.Errorf("InterruptedMessage(1) = %q", msg)
	}
	if msg := InterruptedMessage(nil); msg != "" {
		t.Errorf("InterruptedMessage(nil) = %q, want \"\"", msg)
	}
	t.Log("interrupted filter picks actionable rows in order")
}

func TestRowCompatPrefixShown(t *testing.T) {
	sess := NewSession(Deps{})
	row := sess.toRow(context.Background(), app.LibraryEntry{
		Game: domain.Game{
			Name:         "X",
			Store:        domain.StoreSteam,
			CompatPrefix: "/steam/steamapps/compatdata/100/pfx",
		},
		OptiScalerVersion: "0.9.4",
		ComponentVersions: map[string]string{"dlss": "DLSS 3.7.10", "fsr": "FSR 3.1.4"},
	})
	t.Logf("row: compat=%q optiscaler=%q components=%v", row.CompatPrefix, row.OptiScalerVersion, row.Components)
	if row.CompatPrefix != "/steam/steamapps/compatdata/100/pfx" {
		t.Errorf("CompatPrefix = %q, want carried through", row.CompatPrefix)
	}
	if row.OptiScalerVersion != "0.9.4" {
		t.Errorf("OptiScalerVersion = %q, want %q", row.OptiScalerVersion, "0.9.4")
	}
	want := []string{"DLSS 3.7.10", "FSR 3.1.4"}
	if !slices.Equal(row.Components, want) {
		t.Errorf("Components = %v, want %v (sorted)", row.Components, want)
	}
}
