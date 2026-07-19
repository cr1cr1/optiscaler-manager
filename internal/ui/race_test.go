package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

	// Drain events continuously so the 64-slot buffer never drops, and
	// count scan completions: Scan is fire-and-forget, so without this the
	// test would return with scans in flight and TempDir cleanup would race
	// them. The switch below issues one Scan per (w+i)%4==0 slot: 25 total.
	var scansDone atomic.Int32
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case ev := <-e.sess.Events():
				if ev.Kind == EvScanDone || ev.Kind == EvScanFailed {
					scansDone.Add(1)
				}
			}
		}
	}()

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

	const wantScans = workers * iters / 4
	deadline := time.Now().Add(30 * time.Second)
	for scansDone.Load() < wantScans && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := scansDone.Load(); got < wantScans {
		t.Fatalf("only %d of %d scans finished", got, wantScans)
	}

	got := e.sess.Settings()
	if got.DefaultVersion == "" || got.LaunchTemplate == "" {
		t.Fatalf("settings corrupted under concurrency: %+v", got)
	}
	t.Logf("settings after hammering: version=%q extraDirs=%d",
		got.DefaultVersion, len(got.ExtraDirs))
}
