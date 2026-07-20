package gui

// Temporary artifact generator for the T2 report — renders the four key
// scenes at 800/1100/3840 into /tmp/om-t2-artifacts. Not committed.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

func TestGenArtifacts(t *testing.T) {
	dir := "/tmp/om-t2-artifacts"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	sess, _ := guiFakes(t)
	for _, name := range []string{"Bravo", "Charlie"} {
		d := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		sess.AddDirectory(d)
	}
	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) < 3 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	emptySess, _ := guiFakes(t, func(d *ui.Deps) { d.SteamRoot = t.TempDir() })

	scenes := map[string]func() *model{
		"grid": func() *model {
			m := newModel(Config{Session: sess})
			m.drain()
			return m
		},
		"detail": func() *model {
			m := newModel(Config{Session: sess})
			sess.Select(sess.VisibleRows()[0].InstallDir)
			m.drain()
			return m
		},
		"settings": func() *model {
			m := newModel(Config{Session: sess})
			sess.Select("")
			m.openSettings()
			m.drain()
			return m
		},
		"empty": func() *model {
			m := newModel(Config{Session: emptySess})
			m.drain()
			return m
		},
	}

	for _, scene := range []string{"grid", "detail", "settings", "empty"} {
		for _, v := range []struct{ w, h int }{{800, 600}, {1100, 700}, {3840, 1080}} {
			m := scenes[scene]()
			out := fmt.Sprintf("%s/%s-%d.png", dir, scene, v.w)
			if err := renderToPNG(out, v.w, v.h, m.rootView); err != nil {
				t.Fatalf("%s: %v", out, err)
			}
			st, err := os.Stat(out)
			if err != nil || st.Size() == 0 {
				t.Fatalf("%s missing or empty: %v", out, err)
			}
			t.Logf("%s: %d bytes", out, st.Size())
		}
	}
}
