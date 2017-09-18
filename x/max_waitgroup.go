package x

import "sync"

// Throttle allows a limited number of workers to run at a time. It also
// provides a mechanism to wait for all workers to finish.
type Throttle struct {
	wg sync.WaitGroup
	ch chan struct{}
}

// NewThrottle creates a new throttle with a max number of workers.
func NewThrottle(max int) *Throttle {
	return &Throttle{
		ch: make(chan struct{}, max),
	}
}

// Start should be called by workers before they start working. It blocks if
// there are already the maximum number of workers working.
func (t *Throttle) Start() {
	t.ch <- struct{}{}
	t.wg.Add(1)
}

// Done should be called by workers when they finish working. It panics if
// there wasn't a corresponding Start call.
func (t *Throttle) Done() {
	select {
	case <-t.ch:
	default:
		panic("throttle has no active users")
	}
	t.wg.Done()
}

// Wait waits until all workers have finished working.
func (t *Throttle) Wait() {
	t.wg.Wait()
}
