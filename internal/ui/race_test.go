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
	// count scan completions. Scans serialize and coalesce (a Scan landing
	// mid-scan sets a pending bit), so the 25 Scan calls below no longer
	// map 1:1 to completions; the marker scan after the workers finish
	// proves no request is terminally lost.
	var scansDone atomic.Int32
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case ev := <-e.sess.Events():
				if ev.Kind == EvScanFailed || (ev.Kind == EvScanDone && ev.Text != "directory added") {
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

	// The workers issued 25 Scan calls (one per (w+i)%4==0 slot), but scans
	// coalesce, so the completion count is nondeterministic. Assert liveness
	// instead: a marker Scan issued now must still produce a completion.
	before := scansDone.Load()
	e.sess.Scan(ctx)
	deadline := time.Now().Add(30 * time.Second)
	for scansDone.Load() == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if scansDone.Load() == before {
		t.Fatal("marker scan never completed — a Scan request was lost")
	}
	// Let queued scans settle so TempDir cleanup cannot race them.
	for !e.sess.scanIdle() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	got := e.sess.Settings()
	if got.DefaultVersion == "" || got.LaunchTemplate == "" {
		t.Fatalf("settings corrupted under concurrency: %+v", got)
	}
	t.Logf("settings after hammering: version=%q extraDirs=%d",
		got.DefaultVersion, len(got.ExtraDirs))
}

// TestSessionRemoveDirectoryVsScan hammers RemoveDirectory against
// concurrent Scan: the scan goroutine iterates a settings snapshot's
// ExtraDirs outside the session mutex, so an in-place mutation of the
// shared backing array trips the race detector (found by review).
func TestSessionRemoveDirectoryVsScan(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()

	extraRoot := t.TempDir()
	const iters = 30
	dirs := make([]string, 0, iters)
	for i := 0; i < iters; i++ {
		d := filepath.Join(extraRoot, fmt.Sprintf("Rm%03d", i))
		writeUIFile(t, filepath.Join(d, "game.exe"), "GAME")
		dirs = append(dirs, d)
	}

	ctx := context.Background()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-e.sess.Events():
			}
		}
	}()

	// Pre-seed ExtraDirs so every RemoveDirectory rewrites a large shared
	// backing array in place — widening the window where a concurrent scan
	// snapshot iterates the same memory.
	const seeded = 16
	for i := 0; i < seeded; i++ {
		e.sess.AddDirectory(dirs[i])
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			e.sess.Scan(ctx)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			e.sess.RemoveDirectory(dirs[seeded+i%2])
			e.sess.AddDirectory(dirs[seeded+i%2])
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = e.sess.Settings()
			_ = e.sess.VisibleRows()
		}
	}()
	wg.Wait()

	// Let in-flight and pending scans settle so TempDir cleanup cannot race
	// them.
	deadline := time.Now().Add(10 * time.Second)
	for !e.sess.scanIdle() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("after hammering: extraDirs=%d rows=%d",
		len(e.sess.Settings().ExtraDirs), len(e.sess.VisibleRows()))
}
