// Package ui is the frontend-agnostic interactive core: one Session drives
// the game library, operations, and notifications for ANY frontend (shirei
// GUI, bubbletea TUI, scripted CLI). It contains no display toolkit imports.
package ui

import (
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
	TechBadges   []Badge
	Status       domain.Status
	Actionable   bool
	EAC          bool
	CoverPath    string
	ModTime      time.Time
}

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
