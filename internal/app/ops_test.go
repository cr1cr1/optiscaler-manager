package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRunOps_ConcurrentPerGame_SingleError runs one failing op alongside two
// long-running siblings: all ops must be in flight concurrently, the first
// error wins with game-dir context, siblings are cancelled, and RunOps
// returns exactly one error.
func TestRunOps_ConcurrentPerGame_SingleError(t *testing.T) {
	errBoom := errors.New("boom")

	started := make(chan string, 3)
	allStarted := make(chan struct{})
	siblingCancelled := make(chan string, 2)

	op := func(dir string, fail bool) Op {
		return Op{GameDir: dir, Run: func(ctx context.Context) error {
			started <- dir
			if fail {
				<-allStarted // prove siblings launched before the failure
				return errBoom
			}
			<-ctx.Done() // siblings run until cancelled
			siblingCancelled <- dir
			return ctx.Err()
		}}
	}
	go func() {
		for i := 0; i < 3; i++ {
			<-started
		}
		close(allStarted)
	}()

	err := RunOps(context.Background(),
		op("gameA", true), op("gameB", false), op("gameC", false))

	if !errors.Is(err, errBoom) {
		t.Fatalf("RunOps err = %v, want errors.Is boom", err)
	}
	if !strings.Contains(err.Error(), "gameA") {
		t.Errorf("error %q lacks failing game dir %q", err, "gameA")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("first error must win; got sibling cancellation: %v", err)
	}
	// RunOps only returns after every sibling exited (both observed cancel).
	if len(siblingCancelled) != 2 {
		t.Errorf("siblings cancelled: %d, want 2", len(siblingCancelled))
	}
	t.Logf("single error with game-dir context: %v", err)
}

func TestRunOps_AllSucceed(t *testing.T) {
	ran := make(chan string, 2)
	mk := func(dir string) Op {
		return Op{GameDir: dir, Run: func(ctx context.Context) error {
			ran <- dir
			return nil
		}}
	}
	if err := RunOps(context.Background(), mk("gameA"), mk("gameB")); err != nil {
		t.Fatalf("RunOps: %v", err)
	}
	if len(ran) != 2 {
		t.Errorf("ops ran: %d, want 2", len(ran))
	}
}

func TestRunOps_ParentCancelPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunOps(ctx, Op{GameDir: "gameA", Run: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOps err = %v, want errors.Is context.Canceled", err)
	}
}
