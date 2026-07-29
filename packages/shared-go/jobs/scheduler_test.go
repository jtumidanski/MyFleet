package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEvery_invokesFnAndStopsOnCancel(t *testing.T) {
	var n int32
	ctx, cancel := context.WithCancel(context.Background())
	go Every(ctx, 10*time.Millisecond, func(context.Context) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	time.Sleep(55 * time.Millisecond)
	cancel()
	if atomic.LoadInt32(&n) < 3 {
		t.Fatalf("expected >=3 invocations, got %d", n)
	}
}
