package gui

import (
	"fmt"
	"path/filepath"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/shadercache"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Shader-cache section of the detail panel (issue 078): one row per
// discovered cache with Clear where deletion is safe, the KISS env
// presets, and Warm where a watchdog has something to watch. Rows
// render only when present — an empty game shows the shell and moves
// on. All destructive acts run through the session (per-row clear,
// clear-all, warm); shared driver caches pause on the session's consent
// gate, rendered by the existing confirm modal with zero new plumbing.
func (m *model) shaderSection(e *ui.GameRow) {
	refs := m.sess.ShaderCaches(e.InstallDir)
	txt("Shader caches")
	if len(refs) == 0 {
		muted("No shader caches found")
	}
	claimed := 0
	for i := range refs {
		r := refs[i]
		label := shaderRefLabel(r)
		switch r.Kind {
		case shadercache.KindDXVK, shadercache.KindVKD3D:
			claimed++
			Container(Attrs(Row, Wrap, Gap(sp8), CrossMid), func() {
				muted(label)
				Container(Attrs(Row), func() {
					if i == 0 {
						m.shaderClearRect = GetScreenRectOf(CurrentId())
					}
					if focusableButton(NoIcon, "Clear") {
						m.sess.ClearShaderCache(e.InstallDir, r.Path)
					}
				})
			})
		case shadercache.KindMesa, shadercache.KindNvidia:
			Container(Attrs(Row, Wrap, Gap(sp8), CrossMid), func() {
				muted(label)
				Container(Attrs(Row), func() {
					if focusableButton(NoIcon, "Clear…") {
						m.sess.ClearShaderCache(e.InstallDir, r.Path)
					}
				})
			})
		default:
			muted(label)
		}
	}
	if e.SteamAppID != "" && !hasKind(refs, shadercache.KindSteamFoz) {
		muted("Steam precache: not fetched")
	}
	if claimed > 0 {
		Container(Attrs(Row), func() {
			m.shaderClearAllRect = GetScreenRectOf(CurrentId())
			if focusableButton(NoIcon, "Clear all game caches") {
				m.sess.ClearGameShaderCaches(e.InstallDir)
			}
		})
	}
	m.shaderEnvToggles(e)
	m.shaderWarm(e)
}

// shaderEnvToggles renders the v1 preset knobs over the existing
// per-game env pin: toggles flip on/off, the Mesa cap cycles its
// values. Off stops growth — it never deletes; the label says so.
func (m *model) shaderEnvToggles(e *ui.GameRow) {
	cur := m.sess.UmuEnvFor(e.InstallDir)
	muted("Cache env (takes effect next launch; existing caches kept)")
	for i, p := range ui.ShaderCacheEnvPresets() {
		p := p
		var label, next string
		if len(p.Options) == 0 {
			on := cur[p.Key] == "" || cur[p.Key] == p.On
			label = p.Label + ": " + onOff(on)
			if on {
				next = p.Off
			} else {
				next = p.On
			}
		} else {
			at := 0
			for j, o := range p.Options {
				if cur[p.Key] == o || (cur[p.Key] == "" && o == "default") {
					at = j
					break
				}
			}
			label = p.Label + ": " + p.Options[at]
			next = p.Options[(at+1)%len(p.Options)]
		}
		Container(Attrs(Row), func() {
			if i == 0 {
				m.shaderEnvRect = GetScreenRectOf(CurrentId())
			}
			hoverTip("writes the existing per-game launch env pin")
			if focusableButton(NoIcon, label) {
				m.sess.SetShaderEnv(e.InstallDir, p.Key, next)
			}
		})
	}
}

// shaderWarm renders the launch-and-record button, or the live progress
// while a run owns the game. Availability mirrors the session guard;
// the session re-checks before launching.
func (m *model) shaderWarm(e *ui.GameRow) {
	if prog, ok := m.sess.WarmState(e.InstallDir); ok {
		muted(fmt.Sprintf("Warming shaders: +%s (%s) — Cancel below stops it",
			ui.FormatBytes(prog.BytesGrown), ui.FormatDuration(prog.Elapsed)))
		return
	}
	if !m.sess.HasTrackableShaderCache(e.InstallDir) {
		muted("Warm unavailable (no trackable cache)")
		return
	}
	Container(Attrs(Row), func() {
		m.shaderWarmRect = GetScreenRectOf(CurrentId())
		hoverTip("launches the game and records shader growth until it settles")
		if focusableButton(SymPlay, "Warm now") {
			m.sess.StartWarm(e.InstallDir)
		}
	})
}

func hasKind(refs []shadercache.Ref, k shadercache.Kind) bool {
	for _, r := range refs {
		if r.Kind == k {
			return true
		}
	}
	return false
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// shaderRefLabel is the one-line row text: what, how big, how fresh.
// Dir refs carry the dir mtime (an approximation — discovery documents
// it, the pane does not pretend otherwise).
func shaderRefLabel(r shadercache.Ref) string {
	age := "age unknown"
	if !r.ModTime.IsZero() {
		age = shaderAge(time.Since(r.ModTime)) + " ago"
	}
	size := ui.FormatBytes(r.SizeBytes)
	switch r.Kind {
	case shadercache.KindDXVK:
		return fmt.Sprintf("DXVK state cache · %s · %s", size, age)
	case shadercache.KindVKD3D:
		return fmt.Sprintf("vkd3d-proton cache · %s · %s", size, age)
	case shadercache.KindSteamFoz:
		return fmt.Sprintf("Steam precache · %s · %s", size, age)
	case shadercache.KindUnclaimed:
		return fmt.Sprintf("%s (not managed) · %s", filepath.Base(r.Path), size)
	case shadercache.KindMesa:
		return fmt.Sprintf("Mesa (shared, all games) · %s", size)
	case shadercache.KindNvidia:
		return fmt.Sprintf("NVIDIA (shared, all games) · %s", size)
	default:
		return fmt.Sprintf("%s · %s", r.Path, size)
	}
}

func shaderAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
