package desktop

import (
	"context"
	"sync"
)

// Background owns the application's workers. Register every worker before
// running the application; Stop cancels them and waits for their cleanup.
type Background struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewBackground() *Background {
	ctx, cancel := context.WithCancel(context.Background())
	return &Background{ctx: ctx, cancel: cancel}
}

func (b *Background) Go(work func(context.Context)) {
	b.wg.Go(func() { work(b.ctx) })
}

func (b *Background) Stop() {
	b.cancel()
	b.wg.Wait()
}
