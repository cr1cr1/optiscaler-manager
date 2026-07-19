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
