package ui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedMeas returns Measure funcs over a scripted size series (one
// entry per poll; the last repeats forever).
func scriptedMeas(sizes ...int64) func(string) int64 {
	var calls int64
	return func(string) int64 {
		i := atomic.AddInt64(&calls, 1) - 1
		if int(i) >= len(sizes) {
			return sizes[len(sizes)-1]
		}
		return sizes[i]
	}
}

func testBackend(measure func(string) int64) LaunchWatchBackend {
	launched := true
	return LaunchWatchBackend{
		Launch:        func(string) {},
		Stop:          func(string) bool { return true },
		Launched:      func(string) bool { return launched },
		Measure:       measure,
		Poll:          5 * time.Millisecond,
		QuietNeeded:   3,
		MaxTime:       2 * time.Second,
		LaunchTimeout: 50 * time.Millisecond,
	}
}

// Growth then stillness settles quiet with the grown bytes.
func TestLaunchWatchSettlesQuiet(t *testing.T) {
	b := testBackend(scriptedMeas(100, 200, 300, 300, 300, 300))
	var progress int
	res, err := b.Warm(context.Background(), WarmTarget{Dir: "g"}, func(int64, time.Duration) { progress++ })
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopQuiet || res.BytesGrown != 200 {
		t.Errorf("res = %+v, want quiet +200", res)
	}
	if progress == 0 {
		t.Error("no progress callbacks")
	}
}

// A launch that never reports running fails fast instead of watching
// nothing for twenty minutes.
func TestLaunchWatchLaunchNeverStarts(t *testing.T) {
	b := testBackend(scriptedMeas(0))
	b.Launched = func(string) bool { return false }
	if _, err := b.Warm(context.Background(), WarmTarget{Dir: "g"}, nil); err == nil {
		t.Fatal("expected launch-never-started error")
	}
}

// The game exiting on its own ends the run with what was recorded.
func TestLaunchWatchGameExits(t *testing.T) {
	polls := 0
	b := testBackend(scriptedMeas(100, 160, 160))
	b.Launched = func(string) bool {
		polls++
		return polls < 3
	}
	res, err := b.Warm(context.Background(), WarmTarget{Dir: "g"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopGameExited || res.BytesGrown != 60 {
		t.Errorf("res = %+v, want game-exited +60", res)
	}
}

// Endless growth hits the cap instead of running forever.
func TestLaunchWatchTimeout(t *testing.T) {
	var n int64
	b := testBackend(func(string) int64 { return atomic.AddInt64(&n, 10) })
	b.MaxTime = 40 * time.Millisecond
	res, err := b.Warm(context.Background(), WarmTarget{Dir: "g"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopTimeout || res.BytesGrown <= 0 {
		t.Errorf("res = %+v, want timeout with growth", res)
	}
}

// Cancel mid-run reports cancellation with the caller's error.
func TestLaunchWatchCancel(t *testing.T) {
	var n int64
	b := testBackend(func(string) int64 { return 100 + atomic.AddInt64(&n, 10) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan WarmResult, 1)
	go func() {
		res, err := b.Warm(ctx, WarmTarget{Dir: "g"}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		done <- res
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case res := <-done:
		if res.StopReason != StopCancelled {
			t.Errorf("reason = %q, want cancelled", res.StopReason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not return after cancel")
	}
}

// Clearing mid-run floors growth at zero instead of going negative.
func TestLaunchWatchClearMidRun(t *testing.T) {
	b := testBackend(scriptedMeas(500, 0, 0, 0, 0))
	res, err := b.Warm(context.Background(), WarmTarget{Dir: "g"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.BytesGrown != 0 {
		t.Errorf("grown = %d, want 0 (cleared floor)", res.BytesGrown)
	}
}
