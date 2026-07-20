package gui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// guiFakesWithDirs builds a session whose settings carry extra scan dirs.
func guiFakesWithDirs(t *testing.T, dirs ...string) (*ui.Session, string) {
	t.Helper()
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.Settings = settings.Defaults()
		d.Settings.ExtraDirs = dirs
	})
	return sess, root
}

// TestGUISettingsListsDirectories: the settings modal binds its directory
// section to the session's ExtraDirs and renders a valid frame.
func TestGUISettingsListsDirectories(t *testing.T) {
	dirs := []string{"/games/alpha", "/games/beta"}
	sess, _ := guiFakesWithDirs(t, dirs...)
	m := newModel(Config{Session: sess})
	m.openSettings()

	got := m.settingsDirs()
	if !slices.Equal(got, dirs) {
		t.Fatalf("settingsDirs %v, want session ExtraDirs %v", got, dirs)
	}

	out := filepath.Join(t.TempDir(), "settings.png")
	if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
		t.Fatalf("renderToPNG settings modal: %v", err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("settings modal frame missing or empty: %v", err)
	}
	t.Logf("settings modal lists %d directories", len(got))
}

// TestGUIRemoveDirectoryViaSettings: Tab → Enter on the first remove button
// in the directory section unregisters that directory from the session.
func TestGUIRemoveDirectoryViaSettings(t *testing.T) {
	sess, _ := guiFakesWithDirs(t, "/games/alpha", "/games/beta")
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 700, 500)
	view := func() {
		Container(Attrs(Viewport), func() {
			m.settingsDirsSection()
		})
	}
	keyFrame(KeyCodeNone, 0, view) // build + register focusables
	keyFrame(KeyTab, 0, view)      // focus first row's remove button
	keyFrame(KeyEnter, 0, view)    // activate it

	got := sess.Settings().ExtraDirs
	if slices.Contains(got, "/games/alpha") {
		t.Errorf("first directory still present after remove: %v", got)
	}
	if len(got) != 1 {
		t.Errorf("ExtraDirs %v, want exactly one directory left", got)
	}
	if dirs := m.settingsDirs(); slices.Contains(dirs, "/games/alpha") {
		t.Errorf("settingsDirs still lists removed directory: %v", dirs)
	}
	t.Logf("after remove: %v", got)
}

// TestGUILaunchTemplateEdit: the template input is primed from the session
// and applySettings persists the edit through SetLaunchTemplate.
func TestGUILaunchTemplateEdit(t *testing.T) {
	sess, root := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	if m.templateBuf != sess.Settings().LaunchTemplate {
		t.Fatalf("template buffer %q not primed from session %q", m.templateBuf, sess.Settings().LaunchTemplate)
	}
	m.templateBuf = `"{exe}" -fullscreen {args}`
	m.applySettings()

	if got := sess.Settings().LaunchTemplate; got != m.templateBuf {
		t.Errorf("session launch template %q, want %q", got, m.templateBuf)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after apply: %v", err)
	}
	if s.LaunchTemplate != m.templateBuf {
		t.Errorf("persisted launch template %q, want %q", s.LaunchTemplate, m.templateBuf)
	}
	t.Logf("launch template persisted: %q", s.LaunchTemplate)
}
