package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Defaults seed the two built-in forks: upstream first, the DLSSNR
// multipass fork second; the active fork is upstream.
func TestSettings_ForkDefaults(t *testing.T) {
	s := Defaults()
	if len(s.Forks) < 2 {
		t.Fatalf("Defaults().Forks = %v, want the two built-ins", s.Forks)
	}
	if s.Forks[0].Slug != DefaultForkSlug {
		t.Errorf("first fork %q, want upstream %q", s.Forks[0].Slug, DefaultForkSlug)
	}
	if s.Forks[0].AssetPattern != "Optiscaler_*.7z" {
		t.Errorf("upstream pattern %q, want Optiscaler_*.7z", s.Forks[0].AssetPattern)
	}
	if got := s.Active(); got.Slug != DefaultForkSlug {
		t.Errorf("Active() = %+v, want upstream", got)
	}
	t.Logf("built-in forks: %v", s.Forks)
}

// A legacy settings.json written before forks existed loads with the
// built-ins present and upstream active.
func TestSettings_LegacyJSONWithoutForks(t *testing.T) {
	root := t.TempDir()
	legacy := `{"default_version":"latest","launch_template":"\"{exe}\" {args}"}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	if len(got.Forks) < 2 {
		t.Errorf("Forks = %v, want the built-ins on a legacy file", got.Forks)
	}
	if got.Active().Slug != DefaultForkSlug {
		t.Errorf("Active() = %+v, want upstream on a legacy file", got.Active())
	}
}

// Active resolves the ActiveFork slug to its entry; an unknown or empty
// slug falls back to upstream, and Load normalizes the stored value.
func TestSettings_ActiveForkFallback(t *testing.T) {
	s := Defaults()
	s.Forks = append(s.Forks, Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "OptiScaler*.zip"})
	s.ActiveFork = "someone/OptiScaler-fork"
	if got := s.Active(); got.Slug != "someone/OptiScaler-fork" {
		t.Errorf("Active() = %+v, want the custom fork", got)
	}

	root := t.TempDir()
	s.ActiveFork = "ghost/repo"
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active().Slug != DefaultForkSlug {
		t.Errorf("Active() with a ghost slug = %+v, want upstream", got.Active())
	}
}

// Load guarantees the upstream entry even when a hand-edited file drops
// it, prepending it ahead of the user's custom forks.
func TestSettings_LoadRestoresUpstreamFork(t *testing.T) {
	root := t.TempDir()
	s := Defaults()
	s.Forks = []Fork{{Slug: "someone/OptiScaler-fork", AssetPattern: "*.7z"}}
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Forks) != 2 || got.Forks[0].Slug != DefaultForkSlug {
		t.Errorf("Forks = %v, want upstream prepended", got.Forks)
	}
}

// AddFork validates the slug shape, the pattern, and slug uniqueness.
func TestSettings_AddForkValidation(t *testing.T) {
	base := Defaults()
	cases := []struct {
		name string
		fork Fork
	}{
		{"empty slug", Fork{Slug: "", AssetPattern: "*.7z"}},
		{"no owner/repo shape", Fork{Slug: "noslash", AssetPattern: "*.7z"}},
		{"too many segments", Fork{Slug: "a/b/c", AssetPattern: "*.7z"}},
		{"empty segment", Fork{Slug: "a/", AssetPattern: "*.7z"}},
		{"illegal char", Fork{Slug: "a b/repo", AssetPattern: "*.7z"}},
		{"empty pattern", Fork{Slug: "a/b", AssetPattern: ""}},
		{"pattern without wildcard", Fork{Slug: "a/b", AssetPattern: "OptiScaler.7z"}},
		{"duplicate slug", Fork{Slug: DefaultForkSlug, AssetPattern: "*.zip"}},
	}
	for _, tc := range cases {
		s := base
		s.Forks = append([]Fork(nil), base.Forks...)
		if err := s.AddFork(tc.fork); err == nil {
			t.Errorf("%s: AddFork(%+v) succeeded, want error", tc.name, tc.fork)
		}
	}

	s := base
	ok := Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "OptiScaler*.zip"}
	if err := s.AddFork(ok); err != nil {
		t.Fatalf("AddFork(valid): %v", err)
	}
	if got := s.Forks[len(s.Forks)-1]; got != ok {
		t.Errorf("appended fork = %+v, want %+v", got, ok)
	}
	t.Log("slug shape, pattern wildcard, and duplicate rejection all enforced")
}

// RemoveFork refuses the upstream built-in; removing the active fork
// resets the selection to upstream.
func TestSettings_RemoveFork(t *testing.T) {
	s := Defaults()
	if err := s.RemoveFork(DefaultForkSlug); err == nil {
		t.Error("RemoveFork(upstream) succeeded, want refusal")
	}
	if err := s.RemoveFork("ghost/repo"); err == nil {
		t.Error("RemoveFork(unknown) succeeded, want error")
	}

	custom := Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "*.zip"}
	if err := s.AddFork(custom); err != nil {
		t.Fatal(err)
	}
	s.ActiveFork = custom.Slug
	if err := s.RemoveFork(custom.Slug); err != nil {
		t.Fatalf("RemoveFork(custom): %v", err)
	}
	if got := s.Active(); got.Slug != DefaultForkSlug {
		t.Errorf("Active() after removing the active fork = %+v, want upstream", got)
	}
	for _, f := range s.Forks {
		if f.Slug == custom.Slug {
			t.Errorf("fork %q still present after removal", custom.Slug)
		}
	}
}

// The fork list and active selection survive a save/load round-trip.
func TestSettings_ForkRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := Defaults()
	custom := Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "OptiScaler-NR-*.zip"}
	if err := s.AddFork(custom); err != nil {
		t.Fatal(err)
	}
	s.ActiveFork = custom.Slug
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active() != custom {
		t.Errorf("Active() = %+v, want %+v", got.Active(), custom)
	}
}

// ForkKey maps a slug to a single safe path segment for cache namespacing.
func TestForkKey(t *testing.T) {
	k := ForkKey("optiscaler/OptiScaler")
	if strings.ContainsAny(k, `/\`) {
		t.Errorf("ForkKey %q contains a path separator", k)
	}
	if k == "" || k == "." || k == ".." {
		t.Errorf("ForkKey %q is not a usable directory name", k)
	}
	if ForkKey("a/b") == ForkKey("a_b") {
		t.Errorf("ForkKey collides for distinct slugs a/b and a_b: %q", ForkKey("a/b"))
	}
	t.Logf("upstream key: %s", k)
}
