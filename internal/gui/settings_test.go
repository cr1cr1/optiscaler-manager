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

// TestGUISettingsShowsOnlineLookupsToggle: the General section shows the
// online-lookups toggle primed from the session, and Tab+Enter flips it
// through sess.SetOnlineLookups.
func TestGUISettingsShowsOnlineLookupsToggle(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	if !m.onlineBuf {
		t.Fatal("online-lookups toggle not primed from session settings (default true)")
	}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // open frame: registers focusables
	keyFrame(KeyTab, 0, m.rootView)      // version field
	keyFrame(KeyTab, 0, m.rootView)      // online-lookups toggle
	keyFrame(KeyEnter, 0, m.rootView)    // flip off

	if sess.Settings().OnlineLookups {
		t.Error("OnlineLookups still true after Tab+Enter on the toggle, want flipped to false")
	}
	if m.onlineBuf {
		t.Error("toggle buffer still true after Enter, want flipped")
	}
	t.Log("online-lookups toggle flipped via keyboard")
}

// TestGUISettingsThemedInputs: the settings modal fields are the themed dark
// inputs (the searchInput pattern, not shirei's light TextInputExt): they
// join the Tab focus cycle without stealing focus on open, edit their model
// buffers via FrameInput text/backspace, clear on Esc without closing the
// modal, and applySettings still persists the edited template.
func TestGUISettingsThemedInputs(t *testing.T) {
	sess, root := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	typeFrame := func(text string, key KeyCode) {
		FrameInput.Text = text
		keyFrame(key, 0, m.rootView)
		FrameInput.Text = ""
	}

	version0 := m.versionBuf
	template0 := m.templateBuf

	typeFrame("", KeyCodeNone)  // open frame: registers focusables
	typeFrame("x", KeyCodeNone) // no auto-focus: stray typing edits nothing
	if m.versionBuf != version0 || m.templateBuf != template0 {
		t.Fatalf("settings modal stole focus on open: version %q template %q", m.versionBuf, m.templateBuf)
	}

	typeFrame("", KeyTab) // first focusable in the modal trap = version field
	typeFrame("-test", KeyCodeNone)
	wantV := version0 + "-test"
	if m.versionBuf != wantV {
		t.Fatalf("version buffer %q after typing, want %q (themed append editing)", m.versionBuf, wantV)
	}
	typeFrame("", KeyDeleteBackward)
	if wantB := wantV[:len(wantV)-1]; m.versionBuf != wantB {
		t.Fatalf("version buffer %q after backspace, want %q", m.versionBuf, wantB)
	}

	// Esc clears the field and blurs, but must not close the modal.
	typeFrame("", KeyEscape)
	if m.versionBuf != "" {
		t.Errorf("version buffer %q after Esc, want cleared (themed Esc behavior)", m.versionBuf)
	}
	if !m.settingsOpen {
		t.Fatal("Esc inside a settings field closed the modal; want the field cleared instead")
	}

	// Tab cycle from a blurred state restarts at the top of the modal trap:
	// version → add-directory → template field.
	typeFrame("", KeyTab)
	typeFrame("", KeyTab)
	typeFrame("", KeyTab)
	typeFrame(" -fullscreen", KeyCodeNone)
	wantT := template0 + " -fullscreen"
	if m.templateBuf != wantT {
		t.Fatalf("template buffer %q after typing, want %q (themed append editing)", m.templateBuf, wantT)
	}

	m.applySettings()
	if got := sess.Settings().LaunchTemplate; got != wantT {
		t.Errorf("session launch template %q, want %q", got, wantT)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after apply: %v", err)
	}
	if s.LaunchTemplate != wantT {
		t.Errorf("persisted launch template %q, want %q", s.LaunchTemplate, wantT)
	}
	t.Logf("themed settings inputs edited and persisted: version cleared on Esc, template %q", s.LaunchTemplate)
}
