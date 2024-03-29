package clockwork

import (
	"context"
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
