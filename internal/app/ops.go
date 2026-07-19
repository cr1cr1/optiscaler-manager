package app

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Op is one per-game operation runnable under its own derived context.
type Op struct {
	GameDir string
	Run     func(ctx context.Context) error
}

// RunOps runs ops concurrently, one derived context per op. The first error
// wins: the group context is cancelled, siblings observe ctx.Done, and the
// returned error names the failing game dir. A cancelled parent propagates
// as context.Canceled.
func RunOps(ctx context.Context, ops ...Op) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, op := range ops {
		g.Go(func() error {
			opCtx, stop := context.WithCancel(gctx)
			defer stop()
			if err := op.Run(opCtx); err != nil {
				return fmt.Errorf("%s: %w", op.GameDir, err)
			}
			return nil
		})
	}
	return g.Wait()
}
