package ui

import "github.com/cr1cr1/optiscaler-manager/internal/umu"

// UmuPickerOption is one row of the frontends' umu Proton picker: Value
// is what gets persisted (the runner path, "" for Auto), Label is what
// the user reads.
type UmuPickerOption struct {
	Value string
	Label string
}

// UmuPickerRows composes the shared picker rows for the GUI and TUI:
// Auto first, then the detected runners in scan order (newest first),
// plus a Custom row when the pinned path matches no detected runner so
// an existing config stays visible and re-pickable.
func UmuPickerRows(current string, runners []umu.Runner) []UmuPickerOption {
	rows := []UmuPickerOption{{Value: "", Label: "Auto (newest detected)"}}
	known := current == ""
	for _, r := range runners {
		rows = append(rows, UmuPickerOption{Value: r.Path, Label: r.Name + " (" + r.Source + ")"})
		known = known || r.Path == current
	}
	if !known {
		rows = append(rows, UmuPickerOption{Value: current, Label: "Custom: " + current})
	}
	return rows
}

// UmuPickerLabel is the closed picker's read-out: Auto when nothing is
// pinned, the runner's Name (source) for a known pin, Custom otherwise.
func UmuPickerLabel(current string, runners []umu.Runner) string {
	if current == "" {
		return "Auto (newest detected)"
	}
	for _, r := range runners {
		if r.Path == current {
			return r.Name + " (" + r.Source + ")"
		}
	}
	return "Custom: " + current
}
