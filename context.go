package clockwork

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// contextKey is private to this package so we can ensure uniqueness here. This
// type identifies context values provided by this package.
type contextKey string

// keyClock provides a clock for injecting during tests. If absent, a real clock should be used.
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

// FromContext extracts a clock from the context. If not present, a real clock is returned.
func FromContext(ctx context.Context) Clock {
	if clock, ok := ctx.Value(keyClock).(Clock); ok {
		return clock
	}
	return NewRealClock()
}

func (rc *realClock) WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func (rc *realClock) WithDeadline(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, deadline)
}

func (fc *FakeClock) WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return fc.WithDeadline(parent, fc.Now().Add(timeout))
}

func (fc *FakeClock) WithDeadline(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if cur, ok := parent.Deadline(); ok && cur.Before(deadline) {
		// The current deadline is already sooner than the new one.
		return context.WithCancel(parent)
	}
	ctx := &timerCtx{clock: fc, parent: parent, deadline: deadline, done: make(chan struct{})}
	propagateCancel(parent, ctx)
	dur := deadline.Sub(fc.Now())
	if dur <= 0 {
		ctx.cancel(context.DeadlineExceeded) // deadline has already passed
		return ctx, func() {}
	}
	ctx.Lock()
	defer ctx.Unlock()
	if ctx.err == nil {
		ctx.timer = fc.AfterFunc(dur, func() {
			ctx.cancel(context.DeadlineExceeded)
		})
	}
	return ctx, func() { ctx.cancel(context.Canceled) }
}

// propagateCancel arranges for child to be canceled when parent is.
func propagateCancel(parent context.Context, child *timerCtx) {
	if parent.Done() == nil {
		return // parent is never canceled
	}
	go func() {
		select {
		case <-parent.Done():
			child.cancel(parent.Err())
		case <-child.Done():
		}
	}()
}

type timerCtx struct {
	sync.Mutex

	clock    Clock
	parent   context.Context
	deadline time.Time
	done     chan struct{}

	err   error
	timer Timer
}

func (c *timerCtx) cancel(err error) {
	c.Lock()
	defer c.Unlock()
	if c.err != nil {
		return // already canceled
	}
	c.err = err
	close(c.done)
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) { return c.deadline, true }

func (c *timerCtx) Done() <-chan struct{} { return c.done }

func (c *timerCtx) Err() error { return c.err }

func (c *timerCtx) Value(key interface{}) interface{} { return c.parent.Value(key) }

func (c *timerCtx) String() string {
	return fmt.Sprintf("clock.WithDeadline(%s [%s])", c.deadline, c.deadline.Sub(c.clock.Now()))
}
