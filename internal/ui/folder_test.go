package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Issue 027: OpenGameFolder opens the game's binary dir in the OS file
// manager. The binary dir is the injection dir when the row knows it;
// the game root is the fallback.

func TestOpenGameFolderOpensBinaryDir(t *testing.T) {
	e := newTestEnv(t)
	var opened []string
	e.sess.openFolder = func(dir string) error {
		opened = append(opened, dir)
		return nil
	}
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	// A clean game (no OptiScaler install) still has a folder to open.
	e.sess.OpenGameFolder(e.gameRoot)
	if len(opened) != 1 || opened[0] != e.bin {
		t.Fatalf("opened %v, want [%s] (the binary dir)", opened, e.bin)
	}
	t.Logf("opened %s", opened[0])
}

func TestOpenGameFolderFallsBackToGameRoot(t *testing.T) {
	s := NewSession(Deps{Store: store.New(t.TempDir())})
	root := t.TempDir()
	s.st.Rows = []GameRow{{Title: "G", InstallDir: root}} // no InjectionDir yet
	var opened []string
	s.openFolder = func(dir string) error {
		opened = append(opened, dir)
		return nil
	}

	s.OpenGameFolder(root)
	if len(opened) != 1 || opened[0] != root {
		t.Fatalf("opened %v, want [%s] (game root fallback)", opened, root)
	}
}

func TestOpenGameFolderMissingDirToasts(t *testing.T) {
	s := NewSession(Deps{Store: store.New(t.TempDir())})
	gone := filepath.Join(t.TempDir(), "gone")
	s.st.Rows = []GameRow{{Title: "G", InstallDir: gone}}
	called := false
	s.openFolder = func(string) error { called = true; return nil }

	s.OpenGameFolder(gone)
	if called {
		t.Error("opener ran for a directory that is not on disk")
	}
	if toasts := s.Snapshot().Toasts; len(toasts) == 0 {
		t.Error("no warn toast for a missing game folder")
	}
}

func TestOpenGameFolderOpenerErrorToasts(t *testing.T) {
	s := NewSession(Deps{Store: store.New(t.TempDir())})
	root := t.TempDir()
	s.st.Rows = []GameRow{{Title: "G", InstallDir: root}}
	s.openFolder = func(string) error { return errors.New("boom") }

	s.OpenGameFolder(root)
	if toasts := s.Snapshot().Toasts; len(toasts) == 0 {
		t.Error("no warn toast when the file manager fails to start")
	}
}

func TestOpenGameFolderUnknownGameToasts(t *testing.T) {
	s := NewSession(Deps{Store: store.New(t.TempDir())})
	called := false
	s.openFolder = func(string) error { called = true; return nil }

	s.OpenGameFolder("/no/such/game")
	if called {
		t.Error("opener ran for an unknown game")
	}
	if toasts := s.Snapshot().Toasts; len(toasts) == 0 {
		t.Error("no warn toast for an unknown game")
	}
}
