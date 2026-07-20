package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// progressFixture returns an extra root holding n game directories (each a
// 0644 .exe, accepted without an exec bit on unix).
func progressFixture(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < n; i++ {
		writeUIFile(t, filepath.Join(root, fmt.Sprintf("Game%03d", i), "game.exe"), "GAME")
	}
	return root
}

// TestScan_ProgressLifecycle_NilAfterDone: Progress is observable while the
// scan works and nil once the scan settles.
func TestScan_ProgressLifecycle_NilAfterDone(t *testing.T) {
	e := newSlowCoversEnv(t, 300*time.Millisecond)
	e.sess.deps.Settings.ExtraDirs = []string{progressFixture(t, 2)}

	e.sess.Scan(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	seen := false
	for !seen && time.Now().Before(deadline) {
		if e.sess.Snapshot().Progress != nil {
			seen = true
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !seen {
		t.Fatal("Progress was never non-nil during scan")
	}
	waitEvent(t, e.sess, EvScanDone)
	if got := e.sess.Snapshot().Progress; got != nil {
		t.Fatalf("Progress after EvScanDone = %+v, want nil", got)
	}
	t.Log("progress observable mid-scan, nil after done")
}

// TestScan_ProgressMonotonic: within the pipeline, phases arrive in
// discover→enrich→covers order and Done never decreases inside a phase.
func TestScan_ProgressMonotonic(t *testing.T) {
	e := newSlowCoversEnv(t, 100*time.Millisecond)
	e.sess.deps.Settings.ExtraDirs = []string{progressFixture(t, 3)}

	type sample struct {
		phase string
		done  int
		total int
	}
	var mu sync.Mutex
	var samples []sample
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if p := e.sess.Snapshot().Progress; p != nil {
				mu.Lock()
				samples = append(samples, sample{p.Phase, p.Done, p.Total})
				mu.Unlock()
			}
			time.Sleep(time.Millisecond)
		}
	}()

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	close(stop)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(samples) == 0 {
		t.Fatal("no progress sampled during scan")
	}
	order := map[string]int{"discover": 0, "enrich": 1, "covers": 2}
	lastPhase, lastDone := -1, 0
	coversTicks := 0
	for i, sm := range samples {
		o, ok := order[sm.phase]
		if !ok {
			t.Fatalf("sample %d: unknown phase %q", i, sm.phase)
		}
		if o < lastPhase {
			t.Fatalf("sample %d: phase %q after a later phase (samples: %v)", i, sm.phase, samples)
		}
		if o == lastPhase && sm.done < lastDone {
			t.Fatalf("sample %d: Done decreased %d -> %d in phase %q", i, lastDone, sm.done, sm.phase)
		}
		lastPhase, lastDone = o, sm.done
		if sm.phase == "covers" {
			coversTicks++
		}
	}
	if lastPhase != 2 {
		t.Fatalf("last observed phase = %d, want covers (samples: %v)", lastPhase, samples)
	}
	if coversTicks == 0 {
		t.Fatal("no covers-phase progress sampled")
	}
	t.Logf("%d samples, phase order and Done monotonicity hold", len(samples))
}

// TestScan_ProgressPokesThrottled: progress pokes are throttled — a scan
// over many games emits far fewer EvScanProgress events than items.
func TestScan_ProgressPokesThrottled(t *testing.T) {
	const games = 40
	e := newSlowCoversEnv(t, 10*time.Millisecond)
	e.sess.deps.Settings.ExtraDirs = []string{progressFixture(t, games)}

	pokes := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case ev := <-e.sess.Events():
				if ev.Kind == EvScanProgress {
					pokes++
				}
				if ev.Kind == EvScanDone || ev.Kind == EvScanFailed {
					return
				}
			case <-time.After(30 * time.Second):
				return
			}
		}
	}()

	e.sess.Scan(context.Background())
	<-done
	if pokes == 0 {
		t.Fatal("no progress pokes emitted")
	}
	if pokes >= games {
		t.Fatalf("progress pokes = %d, want far fewer than %d items (unthrottled per-item pokes)", pokes, games)
	}
	t.Logf("%d pokes for %d games (throttled)", pokes, games)
}
