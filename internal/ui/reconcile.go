package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
)

// DetailTruth is the directory-verified install state behind one open
// details pane (issue 079). The manager's cached row status is never the
// truth — this is the game directory's word, reconciled on every
// details-pane open and after every op on the selected game.
type DetailTruth struct {
	Dir string
	app.InstallTruth
}

// TruthLine renders the reconciled game-directory truth as the details
// pane's "Game dir:" line, shared by the GUI panel and the TUI detail
// view. Empty when there is nothing to say.
func TruthLine(t *DetailTruth) string {
	if t == nil || t.Dir == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("Game dir: ")
	ver := ""
	if t.Version != "" {
		ver = " (" + t.Version + ")"
	}
	switch {
	case !t.Managed && t.Hook:
		fmt.Fprintf(&b, "external OptiScaler install detected")
		if t.HookVersion != "" {
			fmt.Fprintf(&b, " (%s)", t.HookVersion)
		}
	case !t.Managed:
		b.WriteString("no OptiScaler files detected")
	case t.Tracked == 0:
		fmt.Fprintf(&b, "managed%s, nothing tracked", ver)
	case t.Changed == 0 && t.Missing == 0:
		fmt.Fprintf(&b, "%d/%d tracked files verified%s", t.Verified, t.Tracked, ver)
	default:
		fmt.Fprintf(&b, "%d changed · %d missing of %d tracked%s — differs from manager records",
			t.Changed, t.Missing, t.Tracked, ver)
	}
	if t.HookDisabled {
		b.WriteString(" · hook disabled")
	} else if t.Managed && t.Tracked > 0 && !t.Hook {
		b.WriteString(" · hook missing")
	}
	if t.ShadowDLSS != "" {
		fmt.Fprintf(&b, " · shadow DLSS set in %s (managed copy in %s)",
			truthRel(t.Dir, t.ShadowDLSS), truthRel(t.Dir, t.DLSSDir))
	}
	return b.String()
}

// truthRel renders dir for the truth line: locations inside the game
// read relative ("OptiScaler"), the game root itself reads as "game
// root", anything unrelatable reads as its base name.
func truthRel(root, dir string) string {
	if rel, err := filepath.Rel(root, dir); err == nil {
		switch {
		case rel == ".":
			return "game root"
		case rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)):
			return rel
		}
	}
	return filepath.Base(dir)
}

// reconcileDetail verifies the selected game against its actual directory
// and publishes the result as State.DetailTruth with an EvDetailTruth
// poke. Hashing runs off the caller's goroutine: a big tracked set must
// never stall Select or an op's terminal event. A generation counter keeps
// only the latest request's result, and only while its game is still the
// selected one — a stale reconcile never overwrites a newer panel.
func (s *Session) reconcileDetail(dir string) {
	if dir == "" {
		return
	}
	row := s.findRow(dir)
	if row == nil {
		return
	}
	injectionDir := row.InjectionDir
	s.mu.Lock()
	s.detailTruthGen++
	gen := s.detailTruthGen
	s.mu.Unlock()
	go func() {
		truth := app.ReconcileInstall(s.deps.Store, dir, injectionDir)
		s.mu.Lock()
		if gen != s.detailTruthGen || s.st.Selected != dir {
			s.mu.Unlock()
			return
		}
		s.st.DetailTruth = &DetailTruth{Dir: dir, InstallTruth: truth}
		s.mu.Unlock()
		s.emit(Event{Kind: EvDetailTruth, GameDir: dir})
	}()
}
