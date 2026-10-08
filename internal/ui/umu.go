package ui

import "github.com/cr1cr1/optiscaler-manager/internal/umu"

// UmuPickerOption is one row of the frontends' umu prefix picker: Value
// is what gets persisted (the prefix path, "" for Auto), Label is what
// the user reads.
type UmuPickerOption struct {
	Value string
	Label string
}

// UmuPickerRows composes the shared picker rows for the GUI and TUI:
// the Auto row first (autoLabel differs per context — the global
// default says "Auto (detected)", a per-game pin says "Auto (inherit)"),
// then the detected prefixes in scan order, plus a Custom row when the
// pinned path matches no detected prefix so an existing config stays
// visible and re-pickable.
func UmuPickerRows(autoLabel, current string, prefixes []umu.Prefix) []UmuPickerOption {
	rows := []UmuPickerOption{{Value: "", Label: autoLabel}}
	known := current == ""
	for _, p := range prefixes {
		rows = append(rows, UmuPickerOption{Value: p.Path, Label: p.Name + " (" + p.Source + ")"})
		known = known || p.Path == current
	}
	if !known {
		rows = append(rows, UmuPickerOption{Value: current, Label: "Custom: " + current})
	}
	return rows
}

// UmuPickerLabel is the closed picker's read-out: the Auto label when
// nothing is pinned, the prefix's Name (source) for a known pin,
// Custom otherwise.
func UmuPickerLabel(autoLabel, current string, prefixes []umu.Prefix) string {
	if current == "" {
		return autoLabel
	}
	for _, p := range prefixes {
		if p.Path == current {
			return p.Name + " (" + p.Source + ")"
		}
	}
	return "Custom: " + current
}
