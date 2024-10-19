package clockwork

import (
	"context"
	"errors"
	"sync"
	"time"
)

// contextKey is private to this package so we can ensure uniqueness here. This
// type identifies context values provided by this package.
type contextKey string

// keyClock provides a clock for injecting during tests. If absent, a real clock
// should be used.
var keyClock = contextKey("clock") // clockwork.Clock

// AddToContext creates a derived context that references the specified clock.
//
// Be aware this doesn't change the behavior of standard library functions, such
// as [context.WithTimeout] or [context.WithDeadline]. For this reason, users
// should prefer passing explicit [clockwork.Clock] variables rather can passing
// the clock via the context.
func AddToContext(ctx context.Context, clock Clock) context.Context {
	return context.WithValue(ctx, keyClock, clock)
}

// FromContext extracts a clock from the context. If not present, a real clock
// is returned.
func FromContext(ctx context.Context) Clock {
	if clock, ok := ctx.Value(keyClock).(Clock); ok {
		return clock
	}
	return NewRealClock()
}

// WithDeadline returns a context with a deadline based on a [FakeClock].
//
// The returned context ignores parent cancelation if the parent was cancelled
// with a [context.DeadlineExceeded] error. Any other error returned by the
// parent is treated normally, cancelling the returned context.
//
// If the parent is cancelled with a [context.DeadlineExceeded] error, the only
// way to then cancel the returned context is by calling the returned
// context.CancelFunc.
func WithDeadline(parent context.Context, clock *FakeClock, t time.Time) (context.Context, context.CancelFunc) {
	return newFakeClockContext(parent, t, clock.newTimerAtTime(t, nil).Chan())
}

// WithTimeout returns a context with a timeout based on a [FakeClock].
//
// The returned context follows the same behaviors as [WithDeadline].
func WithTimeout(parent context.Context, clock *FakeClock, d time.Duration) (context.Context, context.CancelFunc) {
	t, deadline := clock.newTimer(d, nil)
	return newFakeClockContext(parent, deadline, t.Chan())
}

// fakeClockContext implements context.Context, using a fake clock for its
// deadline.
//
// It ignores parent cancellation if the parent is cancelled with
// context.DeadlineExceeded.
type fakeClockContext struct {
	parent   context.Context
	deadline time.Time // The user-facing deadline based on the fake clock's time.

	// Tracks timeout/deadline cancellation.
	timerDone <-chan time.Time

	// Tracks manual calls to the cancel function.
	cancel       func() // Closes cancelCalled wrapped in a sync.Once.
	cancelCalled chan struct{}

	// The user-facing data from the context.Context interface.
	ctxDone chan struct{} // Returned by Done().
	err     error         // nil until ctxDone is ready to be closed.
}

func newFakeClockContext(parent context.Context, deadline time.Time, timer <-chan time.Time) (context.Context, context.CancelFunc) {
	cancelCalled := make(chan struct{})
	ctx := &fakeClockContext{
		parent:       parent,
		deadline:     deadline,
		timerDone:    timer,
		cancelCalled: cancelCalled,
		ctxDone:      make(chan struct{}),
		cancel: sync.OnceFunc(func() {
			close(cancelCalled)
		}),
	}
	ready := make(chan struct{}, 1)
	go ctx.runCancel(ready)
	<-ready // Cancellation goroutine is running.
	return ctx, ctx.cancel
}

func (c *fakeClockContext) Deadline() (time.Time, bool) {
	return c.deadline, true
}

func (c *fakeClockContext) Done() <-chan struct{} {
	return c.ctxDone
}

func (c *fakeClockContext) Err() error {
	<-c.Done() // Don't return the error before it is ready.
	return c.err
}

func (c *fakeClockContext) Value(key any) any {
	return c.parent.Value(key)
}

// runCancel runs the fakeClockContext's cancel goroutine and returns the
// fakeClockContext's cancel function.
//
// fakeClockContext is then cancelled when any of the following occur:
//
//   - The fakeClockContext.done channel is closed by its timer.
//   - The returned CancelFunc is executed.
//   - The fakeClockContext's parent context is cancelled with an error other
//     than context.DeadlineExceeded.
func (c *fakeClockContext) runCancel(ready chan struct{}) {
	parentDone := c.parent.Done()

	// Close ready when done, just in case the ready signal races with other
	// branches of our select statement below.
	defer close(ready)

	var ctxErr error
	for ctxErr == nil {
		select {
		case <-c.timerDone:
			ctxErr = context.DeadlineExceeded

		case <-c.cancelCalled:
			ctxErr = context.Canceled

		case <-parentDone:
			parentDone = nil // This case statement can only fire once.

			if err := c.parent.Err(); !errors.Is(err, context.DeadlineExceeded) {
				// The parent context was canceled with some error otehr than deadline
				// exceeded, so we respect it.
				ctxErr = err
			}
		case ready <- struct{}{}:
			// Signals the cancellation goroutine has begun, in an attempt to minimize
			// race conditions related to goroutine startup time.
			ready = nil // This case statement can only fire once.
		}
	}

	c.setError(ctxErr)
	return
}

func (c *fakeClockContext) setError(err error) {
	c.err = err
	close(c.ctxDone)
}
