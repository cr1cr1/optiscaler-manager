package domain_test

import (
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func TestStoreEnumOnGame(t *testing.T) {
	t.Run("zero value Game is Steam", func(t *testing.T) {
		var g domain.Game
		if g.Store != domain.StoreSteam {
			t.Fatalf("zero-value Game.Store = %v (%s), want StoreSteam", g.Store, g.Store)
		}
		t.Logf("zero-value store: %s", g.Store)
	})

	t.Run("String names", func(t *testing.T) {
		cases := []struct {
			store domain.Store
			want  string
		}{
			{domain.StoreSteam, "Steam"},
			{domain.StoreEpic, "Epic"},
			{domain.StoreGOG, "GOG"},
			{domain.StoreManual, "Manual"},
		}
		for _, tc := range cases {
			if got := tc.store.String(); got != tc.want {
				t.Errorf("Store(%d).String() = %q, want %q", int(tc.store), got, tc.want)
			}
		}
	})

	t.Run("additive fields default empty", func(t *testing.T) {
		var g domain.Game
		if g.AppName != "" || g.ExePath != "" || g.CompatPrefix != "" {
			t.Fatalf("zero-value additive fields not empty: %+v", g)
		}
	})
}
