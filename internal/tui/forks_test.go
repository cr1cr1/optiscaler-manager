package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

const dlssnrForkSlug = "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass"

// The settings screen lists the known OptiScaler sources with the active
// fork marked.
func TestTUISettingsListsForks(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Defaults()
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "OptiScaler sources")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame:\n%s", frame)
	for _, want := range []string{"OptiScaler sources", "optiscaler/OptiScaler", dlssnrForkSlug} {
		if !strings.Contains(frame, want) {
			t.Errorf("settings frame lacks %q:\n%s", want, frame)
		}
	}
	if !strings.Contains(frame, "● "+settings.DefaultForkSlug) {
		t.Errorf("active fork not marked in frame:\n%s", frame)
	}
}

// tab moves list focus to the sources section; j/k move the fork cursor;
// enter activates the fork under the cursor (persisted).
func TestTUISettingsForkSelectActivates(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Defaults()
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "OptiScaler sources")

	sendKey(tm, tea.KeyTab)  // focus the sources list
	sendKey(tm, tea.KeyDown) // DLSSNR row
	sendKey(tm, tea.KeyEnter)

	pollUntil(t, "ActiveFork to switch", func() bool {
		return e.sess.Settings().ActiveFork == dlssnrForkSlug
	})
	if got := e.sess.Settings().ActiveFork; got != dlssnrForkSlug {
		t.Errorf("ActiveFork = %q, want %q", got, dlssnrForkSlug)
	}
}

// With the sources list focused, a opens the slug input, then the pattern
// input; both commits register the fork through the session.
func TestTUISettingsAddFork(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Defaults()
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "OptiScaler sources")

	sendKey(tm, tea.KeyTab) // focus the sources list
	tm.Type("a")
	waitFrame(t, tm, "fork slug:")
	tm.Type("someone/OptiScaler-fork")
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "asset glob:")
	tm.Type("OptiScaler*.zip")
	sendKey(tm, tea.KeyEnter)

	pollUntil(t, "custom fork to register", func() bool {
		for _, f := range e.sess.Settings().Forks {
			if f.Slug == "someone/OptiScaler-fork" && f.AssetPattern == "OptiScaler*.zip" {
				return true
			}
		}
		return false
	})
}

// d on a custom fork asks for inline confirmation; y removes it. d on the
// upstream entry refuses (it stays listed).
func TestTUISettingsRemoveFork(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Defaults()
		_ = d.Settings.AddFork(settings.Fork{Slug: "someone/OptiScaler-fork", AssetPattern: "*.zip"})
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "someone/OptiScaler-fork")

	sendKey(tm, tea.KeyTab) // focus the sources list
	sendKey(tm, tea.KeyDown)
	sendKey(tm, tea.KeyDown) // custom fork row
	tm.Type("d")
	waitFrame(t, tm, "[y/n]")
	tm.Type("y")

	pollUntil(t, "custom fork to disappear", func() bool {
		for _, f := range e.sess.Settings().Forks {
			if f.Slug == "someone/OptiScaler-fork" {
				return false
			}
		}
		return true
	})

	// Upstream is never removable: d on the first row keeps it.
	sendKey(tm, tea.KeyUp)
	sendKey(tm, tea.KeyUp)
	tm.Type("d")
	pollUntil(t, "upstream refusal to settle", func() bool {
		for _, toast := range e.sess.Snapshot().Toasts {
			if strings.Contains(toast.Text, "cannot be removed") {
				return true
			}
		}
		return false
	})
	if !strings.Contains(e.sess.Settings().Forks[0].Slug, settings.DefaultForkSlug) {
		t.Error("upstream fork vanished after a refused remove")
	}
}
