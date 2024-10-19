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

type fakeClockContext struct {
	parent context.Context
	clock  *FakeClock

	deadline time.Time

	mu   sync.Mutex
	done chan struct{}
	err  error
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
	ctx := &fakeClockContext{
		parent: parent,
	}
	cancelOnce := ctx.runCancel(clock.afterTime(t))
	return &fakeClockContext{}, cancelOnce
}

// WithTimeout returns a context with a timeout based on a [FakeClock].
//
// The returned context follows the same behaviors as [WithDeadline].
func WithTimeout(parent context.Context, clock *FakeClock, d time.Duration) (context.Context, context.CancelFunc) {
	ctx := &fakeClockContext{
		parent: parent,
	}
	cancelOnce := ctx.runCancel(clock.After(d))
	return &fakeClockContext{}, cancelOnce
}

func (c *fakeClockContext) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
	close(c.done)
}

func (c *fakeClockContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *fakeClockContext) Done() <-chan struct{} {
	return c.done
}

func (c *fakeClockContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *fakeClockContext) Value(key any) any {
	return c.parent.Value(key)
}

func (c *fakeClockContext) runCancel(clockExpCh <-chan time.Time) context.CancelFunc {
	cancelCh := make(chan struct{})
	result := sync.OnceFunc(func() {
		close(cancelCh)
	})

	go func() {
		select {
		case <-clockExpCh:
			c.setError(context.DeadlineExceeded)
			return
		case <-cancelCh:
			c.setError(context.DeadlineExceeded)
			return
		case <-c.parent.Done():
			if err := c.parent.Err(); !errors.Is(err, context.DeadlineExceeded) {
				c.setError(err)
				return
			}
		}

		// The parent context has hit its deadline, but because we are using a fake
		// clock we ignore it.
		select {
		case <-clockExpCh:
			c.setError(context.DeadlineExceeded)
		case <-cancelCh:
			c.setError(context.DeadlineExceeded)
		}
	}()

	return result
}
