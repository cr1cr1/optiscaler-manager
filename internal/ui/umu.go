package ui

import "github.com/cr1cr1/optiscaler-manager/internal/umu"

// UmuPickerOption is one row of the frontends' umu prefix picker: Value
// is what gets persisted (the prefix path, "" for Auto), Source is the
// owning launcher's id ("" for Auto, "custom" for an undetected pin),
// Name the prefix name (the Auto row's label). Source and Name stay
// separate so the frontends can render "icon + Launcher: prefix"
// (issue 059).
type UmuPickerOption struct {
	Value  string
	Source string
	Name   string
}

// UmuPickerRows composes the shared picker rows for the GUI and TUI:
// the Auto row first (autoLabel differs per context — the global
// default says "Auto (detected)", a per-game pin says "Auto (inherit)"),
// then the detected prefixes in scan order, plus a Custom row when the
// pinned path matches no detected prefix so an existing config stays
// visible and re-pickable.
func UmuPickerRows(autoLabel, current string, prefixes []umu.Prefix) []UmuPickerOption {
	rows := []UmuPickerOption{{Value: "", Source: "", Name: autoLabel}}
	known := current == ""
	for _, p := range prefixes {
		rows = append(rows, UmuPickerOption{Value: p.Path, Source: p.Source, Name: p.Name})
		known = known || p.Path == current
	}
	if !known {
		rows = append(rows, UmuPickerOption{Value: current, Source: "custom", Name: current})
	}
	return rows
}

// UmuPickerRowLabel is a row's text form: "Bottles: mybottle" — the
// Auto row renders its label verbatim.
func UmuPickerRowLabel(o UmuPickerOption) string {
	if o.Source == "" {
		return o.Name
	}
	return UmuSourceName(o.Source) + ": " + o.Name
}

// UmuPickerLabel is the closed picker's read-out: the Auto label when
// nothing is pinned, "Launcher: prefix" for a known pin, Custom
// otherwise.
func UmuPickerLabel(autoLabel, current string, prefixes []umu.Prefix) string {
	if current == "" {
		return autoLabel
	}
	for _, p := range prefixes {
		if p.Path == current {
			return UmuSourceName(p.Source) + ": " + p.Name
		}
	}
	return "Custom: " + current
}

// UmuSourceName maps a prefix source id to its display name; unknown
// ids render verbatim.
func UmuSourceName(source string) string {
	switch source {
	case "steam":
		return "Steam"
	case "bottles":
		return "Bottles"
	case "lutris":
		return "Lutris"
	case "heroic":
		return "Heroic"
	case "playonlinux":
		return "PlayOnLinux"
	case "crossover":
		return "CrossOver"
	case "wine":
		return "Wine"
	case "custom":
		return "Custom"
	}
	return source
}
