package worker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrLimiterConcurrency = errors.New("error running task, reached concurrency limit")
	ErrLimiterDrain       = errors.New("draining tasks")
)

// requirements
// - limit queuing more items based on concurrency value
// - drain blocks adding more items to be run, waits until all items are complete
// - uses waitgroup
// - accepts func() - all error handling must be wrapped in a closure by the caller
// - supports returning number of running items

// Limiter runs go routines limiting them by the defined concurrency.
type Limiter struct {
	// waitgroup for running routines.
	wg *sync.WaitGroup
	// routines spawned return once complete on this channel.
	doneCh chan struct{}
	// dispatcherCh is where the dispatch() method listens for funcs to run.
	dispatcherCh chan func()
	// concurrency is the maximum number of goroutines that can be running.
	concurrency int
	// mu is the guard for dispatched, drain.
	mu sync.RWMutex
	// dispatched indicates the number of routines dispatched by this limiter.
	dispatched int32
	// drain is the flag set when StopWait() invoked, with drain=true, no further tasks are accepted.
	drain bool
}

// NewLimiter returns a new limiting go routine runner.
// To ensure the routines spawned by Limiter are stopped, the StopWait() method should be invoked.
//
// concurrency is the limit on the number of running go routines
// this limiter is to ensure
func NewLimiter(concurrency int) *Limiter {
	l := &Limiter{
		concurrency:  concurrency,
		wg:           &sync.WaitGroup{},
		doneCh:       make(chan struct{}),
		dispatcherCh: make(chan func()),
	}

	l.wg.Add(1)

	go l.dispatcher()

	return l
}

// Dispatch dispatches the given routine for execution
//
// The routine to be executed should be wrapped in a closure.
func (l *Limiter) Dispatch(f func()) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if int(l.dispatched) >= l.concurrency {
		return ErrLimiterConcurrency
	}

	if l.drain {
		return ErrLimiterDrain
	}

	l.dispatcherCh <- f

	return nil
}

// dispatcher runs in a loop dispatching routines received over dispatcherCh for execution
// this method returns - once drain is set and dispatched goroutines is zero.
func (l *Limiter) dispatcher() {
	defer l.wg.Done()

	stopWaitCheck := time.NewTicker(time.Millisecond * 200).C

Loop:
	for {
		select {
		case f := <-l.dispatcherCh:
			l.wg.Add(1)
			go func() {
				defer l.wg.Done()
				f()
				l.doneCh <- struct{}{}
			}()

			atomic.AddInt32(&l.dispatched, 1)

		case <-l.doneCh:
			atomic.AddInt32(&l.dispatched, ^int32(0))

		case <-stopWaitCheck:
			l.mu.RLock()

			if l.dispatched == 0 && l.drain {
				break Loop
			}

			l.mu.RUnlock()
		}
	}
}

// ActiveCount returns the count of running routines
func (l *Limiter) ActiveCount() int {
	return int(l.dispatched)
}

// StopWait prevents any further routines from being added
// and waits until all the routines complete.
func (l *Limiter) StopWait() {
	l.mu.Lock()

	l.drain = true
	l.mu.Unlock()

	l.wg.Wait()
}
