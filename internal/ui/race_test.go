package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestSessionSettingsConcurrentAccess hammers every settings-holding path
// (Scan, AddDirectory, SetDefaultVersion, SetLaunchTemplate) concurrently.
// Any settings read that bypasses the session mutex trips the race detector
// (`go test -race`); with proper locking the run is clean.
func TestSessionSettingsConcurrentAccess(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	// One unique manual game dir per AddDirectory call: every call appends
	// to settings.ExtraDirs, so writes stay hot for the whole run.
	extraRoot := t.TempDir()
	const workers = 4
	const iters = 25
	dirs := make([]string, 0, workers*iters)
	for i := 0; i < workers*iters; i++ {
		d := filepath.Join(extraRoot, fmt.Sprintf("Extra%03d", i))
		writeUIFile(t, filepath.Join(d, "game.exe"), "GAME")
		dirs = append(dirs, d)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				switch (w + i) % 4 {
				case 0:
					e.sess.Scan(ctx)
				case 1:
					e.sess.AddDirectory(dirs[w*iters+i])
				case 2:
					e.sess.SetDefaultVersion(fmt.Sprintf("v%d.%d", w, i))
				case 3:
					e.sess.SetLaunchTemplate(fmt.Sprintf(`"{exe}" --w%d-i%d {args}`, w, i))
				}
				_ = e.sess.Settings() // locked snapshot read
			}
		}(w)
	}
	wg.Wait()

	got := e.sess.Settings()
	if got.DefaultVersion == "" || got.LaunchTemplate == "" {
		t.Fatalf("settings corrupted under concurrency: %+v", got)
	}
	t.Logf("settings after hammering: version=%q extraDirs=%d",
		got.DefaultVersion, len(got.ExtraDirs))
}
