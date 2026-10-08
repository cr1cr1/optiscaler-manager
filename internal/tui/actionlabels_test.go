package tui

import (
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 033: the TUI detail actions carry the same labels as the GUI
// buttons — install/uninstall names OptiScaler, launch is "launch game".
func TestDetailActionLabelsNameOptiScaler(t *testing.T) {
	out := sgrRE.ReplaceAllString(detailModelFor(t, ui.GameRow{Title: "G", InstallDir: "/g/one"}).detailView(100, 40), "")
	if !strings.Contains(out, "install/uninstall OptiScaler") {
		t.Errorf("clean row actions missing %q:\n%s", "install/uninstall OptiScaler", out)
	}
	if !strings.Contains(out, "  l  launch game") {
		t.Errorf("actions missing %q:\n%s", "  l  launch game", out)
	}

	ext := sgrRE.ReplaceAllString(detailModelFor(t, ui.GameRow{Title: "G", InstallDir: "/g/one", Status: domain.StatusExternal}).detailView(100, 40), "")
	if !strings.Contains(ext, "adopt OptiScaler") {
		t.Errorf("external row actions missing %q:\n%s", "adopt OptiScaler", ext)
	}
}
