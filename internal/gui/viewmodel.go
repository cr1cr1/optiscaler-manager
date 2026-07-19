// Package gui is the go-shirei frontend: the only package allowed to import
// shirei. The view model (this file) is shirei-free so behavior is testable
// without pixels; views live in view.go.
package gui

import (
	"sort"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Config carries the dependencies Run needs, handed over from cmd.
type Config struct {
	Store     *store.Store
	GH        *gh.Client
	CacheDir  string
	SteamRoot string
	AuditGrid bool
}

// model is the entire UI state. It is only ever mutated on the frame
// goroutine, or under WithFrameLock from background workers.
type model struct {
	cfg        Config
	rows       []app.LibraryEntry
	filter     string
	selected   string // InstallDir of the game whose dashboard is open
	eacPending string // InstallDir awaiting anti-cheat confirmation
	busy       bool
	status     string
	auditGrid  bool
}

func newModel(cfg Config) *model {
	return &model{cfg: cfg, auditGrid: cfg.AuditGrid, status: "Scanning…"}
}

// selectedEntry resolves the selected game inside rows (pointer into the
// slice, so status updates stick).
func (m *model) selectedEntry() *app.LibraryEntry {
	for i := range m.rows {
		if m.rows[i].Game.InstallDir == m.selected {
			return &m.rows[i]
		}
	}
	return nil
}

// installDecision is what should happen when the user clicks Install.
type installDecision int

const (
	installNow installDecision = iota
	confirmEAC
)

// decideInstall routes EAC-protected games through the confirmation modal.
func decideInstall(e app.LibraryEntry) installDecision {
	if e.EAC {
		return confirmEAC
	}
	return installNow
}

// actionable marks installs that need attention (interrupted or failed).
func actionable(s domain.Status) bool {
	return s == domain.StatusFailed || s == domain.StatusInProgress
}

// sortRows orders the library: actionable installs first, then most recently
// touched game directories, then name.
func sortRows(rows []app.LibraryEntry) {
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := actionable(rows[i].Status), actionable(rows[j].Status)
		if ai != aj {
			return ai
		}
		if !rows[i].ModTime.Equal(rows[j].ModTime) {
			return rows[i].ModTime.After(rows[j].ModTime)
		}
		return rows[i].Game.Name < rows[j].Game.Name
	})
}

// filterRows narrows the library by a case-insensitive substring of the game
// name or an appid prefix. An empty query returns the rows unchanged.
func filterRows(rows []app.LibraryEntry, query string) []app.LibraryEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return rows
	}
	out := make([]app.LibraryEntry, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Game.Name), q) || strings.Contains(r.Game.AppID, q) {
			out = append(out, r)
		}
	}
	return out
}

// statusText renders the install status column/badge for one entry.
func statusText(e app.LibraryEntry) string {
	if e.Status == "" {
		return "not installed"
	}
	return string(e.Status)
}
