// Package jobs provides a ticker primitive for background sweeps. Run the fn
// body under database.WithLeaderLock for multi-replica safety (design A9).
package jobs

import (
	"context"
	"time"
)

func Every(ctx context.Context, interval time.Duration, fn func(context.Context) error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = fn(ctx)
		}
	}
}
