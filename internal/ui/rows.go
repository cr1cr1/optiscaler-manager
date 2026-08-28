// Package ui is the frontend-agnostic interactive core: one Session drives
// the game library, operations, and notifications for ANY frontend (shirei
// GUI, bubbletea TUI, scripted CLI). It contains no display toolkit imports.
package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// Tone is a badge color hint; each frontend maps tones to its own palette.
type Tone int

const (
	ToneGray Tone = iota
	ToneGreen
	ToneRed
	ToneBlue
	TonePurple
)

// Badge is a small display pill (technology, platform, status).
type Badge struct {
	Label string
	Tone  Tone
}

// GameRow is one display-ready library entry. Frontends render it verbatim;
// all derivation happens here.
type GameRow struct {
	Title        string
	AppID        string
	InstallDir   string
	InjectionDir string
	Platform     string
	TechBadges   []Badge
	Status       domain.Status
	Actionable   bool
	EAC          bool
	// Disabled reports the install's injection hook renamed to
	// <name>.disabled: OptiScaler is present but the game will not load
	// it. The Disable/Enable toggle flips it.
	Disabled bool
	CoverPath    string
	ModTime      time.Time
	SteamAppID   string // resolved via Steam search or copied from a numeric AppID; "" when unknown
	TitleSource  string // which identification rule produced Title (domain.TitleSource); "" for store rows/legacy
	ProtonTier   string // ProtonDB tier (platinum/gold/silver/bronze/borked); "" when unknown

	Store             domain.Store // raw storefront (launch needs it)
	AppName           string       // Epic launch AppName; "" elsewhere
	ExePath           string       // resolved main executable; "" when unknown
	CompatPrefix      string       // Proton prefix (linux only); "" when absent
	OptiScalerVersion string       // "" when not installed or unknown
	Components        []string     // marketing names, e.g. ["DLSS 3.7.10","FSR 3.1.4"]
}

// SortMode selects the row ordering VisibleRows applies.
type SortMode int

const (
	// SortDefault orders actionable installs first, then recent, then title.
	SortDefault SortMode = iota
	// SortName orders alphabetically by title.
	SortName
)

// badgeForTech maps a classified upscaler kind to its display badge.
func badgeForTech(kind string) Badge {
	switch {
	case strings.HasPrefix(kind, "DLSS"):
		return Badge{Label: kind, Tone: ToneGreen}
	case kind == "FSR":
		return Badge{Label: kind, Tone: ToneRed}
	case kind == "XeSS":
		return Badge{Label: kind, Tone: ToneBlue}
	default:
		return Badge{Label: kind, Tone: ToneGray}
	}
}

// HasInstall reports whether the row carries an OptiScaler install:
// manager-committed or detected external. Anything else has no hook to
// toggle, no ini worth opening, and no version to switch from.
func (r GameRow) HasInstall() bool {
	return r.Status == domain.StatusCommitted || r.Status == domain.StatusExternal
}

// CanOpenINI reports whether the row has an OptiScaler install whose ini is
// worth opening: a manager-committed install or an external one detected on
// disk. Failed, in-progress, rolled-back, and never-installed rows have no
// usable ini, so the affordance stays closed for them.
func (r GameRow) CanOpenINI() bool {
	return r.HasInstall()
}

// DisableToggleLabel is the caption for the hook disable toggle: Enable
// when the hook is renamed away, Disable when it is active. ok=false when
// the row has no install to toggle.
func (r GameRow) DisableToggleLabel() (label string, ok bool) {
	if !r.HasInstall() {
		return "", false
	}
	if r.Disabled {
		return "Enable OptiScaler", true
	}
	return "Disable OptiScaler", true
}

// InterruptedRows filters rows down to interrupted installs (in_progress /
// failed, the actionable set) in order. Frontends build the persistent
// repair/rollback/retry surface from it; the rows themselves carry the
// Rollback and Install affordances.
func InterruptedRows(rows []GameRow) []GameRow {
	var out []GameRow
	for _, r := range rows {
		if r.Actionable {
			out = append(out, r)
		}
	}
	return out
}

// InterruptedMessage is the repair guidance for interrupted installs; the
// boot toast and the frontends' persistent banner share the wording. rows
// must be the InterruptedRows output. "" for an empty set.
func InterruptedMessage(rows []GameRow) string {
	switch len(rows) {
	case 0:
		return ""
	case 1:
		return "interrupted install: " + rows[0].Title + ", rollback to restore or install to retry"
	}
	return fmt.Sprintf("%d interrupted installs: rollback to restore or install to retry", len(rows))
}

// actionableStatus marks installs that need attention (interrupted, failed).
func actionableStatus(s domain.Status) bool {
	return s == domain.StatusFailed || s == domain.StatusInProgress
}

// sortRows orders actionable installs first, then most recently touched,
// then title. The input slice is sorted in place and returned.
func sortRows(rows []GameRow) []GameRow {
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := actionableStatus(rows[i].Status), actionableStatus(rows[j].Status)
		if ai != aj {
			return ai
		}
		if !rows[i].ModTime.Equal(rows[j].ModTime) {
			return rows[i].ModTime.After(rows[j].ModTime)
		}
		return rows[i].Title < rows[j].Title
	})
	return rows
}

// filterRows narrows rows by a case-insensitive substring of the title or an
// appid substring. Empty query returns rows unchanged.
func filterRows(rows []GameRow, query string) []GameRow {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return rows
	}
	out := make([]GameRow, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Title), q) || strings.Contains(r.AppID, q) {
			out = append(out, r)
		}
	}
	return out
}
