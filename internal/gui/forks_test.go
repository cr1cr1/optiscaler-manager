package gui

import (
	"os"
	"path/filepath"
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// The settings modal's OptiScaler Sources section lists the built-in
// forks (upstream active) and renders a valid frame.
func TestGUISettingsForksSectionListsBuiltins(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	forks := sess.Settings().Forks
	if len(forks) != 2 {
		t.Fatalf("built-in forks = %v, want 2", forks)
	}
	if got := sess.Settings().ActiveFork; got != settings.DefaultForkSlug {
		t.Fatalf("active fork = %q, want upstream", got)
	}

	out := filepath.Join(t.TempDir(), "settings-forks.png")
	if err := renderToPNG(out, 1000, 800, m.rootView); err != nil {
		t.Fatalf("renderToPNG settings modal: %v", err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("settings modal frame missing or empty: %v", err)
	}
}

// Tab → Enter on the DLSSNR fork's "Use" button selects it as the active
// fork through the session (persisted).
func TestGUISettingsForkSelectActivates(t *testing.T) {
	sess, root := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 700, 500)
	view := func() {
		Container(Attrs(Viewport), func() {
			m.settingsForksSection()
		})
	}
	keyFrame(KeyCodeNone, 0, view) // build + register focusables
	// The active (upstream) row renders no "Use" button, so the first
	// focusable is the DLSSNR row's "Use".
	keyFrame(KeyTab, 0, view)      // DLSSNR row's "Use" button
	keyFrame(KeyEnter, 0, view)    // arm it
	keyFrame(KeyCodeNone, 0, view) // release fires it (press->release)

	want := "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass"
	if got := sess.Settings().ActiveFork; got != want {
		t.Errorf("ActiveFork = %q after selecting the DLSSNR fork, want %q", got, want)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.ActiveFork != want {
		t.Errorf("persisted ActiveFork = %q, want %q", s.ActiveFork, want)
	}
}

// The add inputs plus the Add button register a custom fork through the
// session and clear the buffers; the new fork's row then offers a Remove
// button that deletes it.
func TestGUISettingsAddAndRemoveFork(t *testing.T) {
	sess, root := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	m.forkSlugBuf = "someone/OptiScaler-fork"
	m.forkPatternBuf = "OptiScaler*.zip"
	m.addForkFromBuffers()

	custom := settings.Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "OptiScaler*.zip"}
	found := false
	for _, f := range sess.Settings().Forks {
		if f == custom {
			found = true
		}
	}
	if !found {
		t.Fatalf("forks = %v, want the added fork", sess.Settings().Forks)
	}
	if m.forkSlugBuf != "" || m.forkPatternBuf != "" {
		t.Errorf("add buffers = %q/%q after a successful add, want cleared", m.forkSlugBuf, m.forkPatternBuf)
	}
	if s, err := settings.Load(root); err != nil || !containsFork(s.Forks, custom) {
		t.Errorf("persisted forks = %v (%v), want the custom fork", s.Forks, err)
	}

	// Invalid input refuses and keeps the buffers for editing.
	m.forkSlugBuf = "noslash"
	m.forkPatternBuf = "*.7z"
	m.addForkFromBuffers()
	if m.forkSlugBuf != "noslash" {
		t.Errorf("slug buffer = %q after a refused add, want kept for editing", m.forkSlugBuf)
	}
	if n := len(sess.Settings().Forks); n != 3 {
		t.Errorf("fork count = %d after a refused add, want 3", n)
	}

	// Remove the custom fork through the session-facing helper the row
	// button calls.
	m.removeFork(custom.Slug)
	if containsFork(sess.Settings().Forks, custom) {
		t.Errorf("forks = %v after remove, want the custom fork gone", sess.Settings().Forks)
	}

	// Upstream is never removable.
	m.removeFork(settings.DefaultForkSlug)
	if !containsFork(sess.Settings().Forks, settings.Fork{Slug: settings.DefaultForkSlug, AssetPattern: "Optiscaler_*.7z"}) {
		t.Error("upstream fork vanished after a refused remove")
	}
}

func containsFork(forks []settings.Fork, want settings.Fork) bool {
	for _, f := range forks {
		if f == want {
			return true
		}
	}
	return false
}
