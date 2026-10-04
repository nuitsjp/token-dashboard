package desktop

import (
	"context"
	"testing"
	"time"
)

func TestBackgroundStopWaitsForEveryWorkerCleanup(t *testing.T) {
	background := NewBackground()
	cancelled := make(chan struct{}, 4)
	cleanup := make(chan struct{})
	finished := make(chan struct{}, 4)
	for range 4 {
		background.Go(func(ctx context.Context) {
			<-ctx.Done()
			cancelled <- struct{}{}
			<-cleanup
			finished <- struct{}{}
		})
	}
	stopped := make(chan struct{})
	go func() {
		background.Stop()
		close(stopped)
	}()
	for range 4 {
		select {
		case <-cancelled:
		case <-time.After(5 * time.Second):
			close(cleanup)
			t.Fatal("a worker was not cancelled")
		}
	}
	select {
	case <-stopped:
		close(cleanup)
		t.Fatal("stop returned while worker cleanup was still pending")
	default:
	}
	close(cleanup)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after cleanup")
	}
	if len(finished) != 4 {
		t.Fatal("not all workers finished cleanup")
	}
	background.Stop() // The shutdown hook and deferred cleanup both stop it.
}
