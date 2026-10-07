package gui

import (
	"context"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 028: the detail panel's title edit flow pre-fills the row title
// and applies the override through the session on Apply; cancelling keeps
// the row untouched.
func TestTitleEditApplyRenamesRow(t *testing.T) {
	settingsRoot := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) { d.SettingsRoot = settingsRoot })
	sess.Start(context.Background())
	rows := waitScanSettled(t, sess, 1)
	row := rows[0]

	m := newModel(Config{Session: sess})
	m.startTitleEdit(row)
	if m.titleEditDir != row.InstallDir || m.titleBuf != row.Title {
		t.Fatalf("title edit state = (%q, %q), want (%q, %q)",
			m.titleEditDir, m.titleBuf, row.InstallDir, row.Title)
	}

	m.titleBuf = "Renamed Title"
	m.applyTitleEdit()
	if m.titleEditDir != "" {
		t.Error("apply did not close the title edit")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := ""
		for _, r := range sess.Snapshot().Rows {
			if r.InstallDir == row.InstallDir {
				got = r.Title
			}
		}
		if got == "Renamed Title" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("row title = %q, want Renamed Title", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	loaded, err := settings.Load(settingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TitleOverrides[row.InstallDir] != "Renamed Title" {
		t.Errorf("persisted TitleOverrides = %v", loaded.TitleOverrides)
	}

	// Cancel path: editing then cancelling leaves the row title alone.
	m.startTitleEdit(row)
	m.titleBuf = "Never Applied"
	m.cancelTitleEdit()
	if m.titleEditDir != "" {
		t.Error("cancel did not close the title edit")
	}
	for _, r := range sess.Snapshot().Rows {
		if r.InstallDir == row.InstallDir && r.Title != "Renamed Title" {
			t.Errorf("cancel changed the row title to %q", r.Title)
		}
	}
}
