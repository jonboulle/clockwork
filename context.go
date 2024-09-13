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

// ClockedContext is a interface that extends the context.Context interface with
// methods for creating new contexts with timeouts and deadlines with a controlled clock.
type ClockedContext interface {
	context.Context
	WithTimeout(parent context.Context, timeout time.Duration) (ClockedContext, context.CancelFunc)
	WithDeadline(parent context.Context, deadline time.Time) (ClockedContext, context.CancelFunc)
}

// WrapContext creates a new context that uses the provided clock for timeouts and deadlines.
func WrapContext(parent context.Context, clock Clock) ClockedContext {
	ctx := &timerCtx{
		clock:  clock,
		parent: parent,
		done:   make(chan struct{}),
	}
	propagateCancel(parent, ctx)
	return ctx
}

func (c *timerCtx) WithTimeout(parent context.Context, timeout time.Duration) (ClockedContext, context.CancelFunc) {
	return c.WithDeadline(parent, c.clock.Now().Add(timeout))
}

func (c *timerCtx) WithDeadline(parent context.Context, deadline time.Time) (ClockedContext, context.CancelFunc) {
	if cur, ok := parent.Deadline(); ok && cur.Before(deadline) {
		// The current deadline is already sooner than the new one.
		return c, func() { c.cancel(parent.Err()) }
	}
	dur := deadline.Sub(c.clock.Now())
	if dur <= 0 {
		c.cancel(context.DeadlineExceeded) // deadline has already passed
		return c, func() {}
	}
	c.Lock()
	defer c.Unlock()
	if c.err == nil {
		c.timer = c.clock.AfterFunc(dur, func() {
			c.cancel(context.DeadlineExceeded)
		})
	}
	return c, func() { c.cancel(context.Canceled) }
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

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) { return c.deadline, !c.deadline.IsZero() }

func (c *timerCtx) Done() <-chan struct{} { return c.done }

func (c *timerCtx) Err() error { return c.err }

func (c *timerCtx) Value(key interface{}) interface{} { return c.parent.Value(key) }

func (c *timerCtx) String() string {
	return fmt.Sprintf("clock.WithDeadline(%s [%s])", c.deadline, c.deadline.Sub(c.clock.Now()))
}
