// Package clockwork contains a simple fake clock for Go.
package clockwork

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Clock provides an interface that packages can use instead of directly using
// the [time] module, so that chronology-related behavior can be tested.
type Clock interface {
	After(d time.Duration) <-chan time.Time
	Sleep(d time.Duration)
	Now() time.Time
	Since(t time.Time) time.Duration
	Until(t time.Time) time.Duration
	NewTicker(d time.Duration) Ticker
	NewTimer(d time.Duration) Timer
	AfterFunc(d time.Duration, f func()) Timer
}

// NewRealClock returns a Clock which simply delegates calls to the actual time
// package; it should be used by packages in production.
func NewRealClock() Clock {
	return &realClock{}
}

type realClock struct{}

func (rc *realClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (rc *realClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (rc *realClock) Now() time.Time {
	return time.Now()
}

func (rc *realClock) Since(t time.Time) time.Duration {
	return rc.Now().Sub(t)
}

func (rc *realClock) Until(t time.Time) time.Duration {
	return t.Sub(rc.Now())
}

func (rc *realClock) NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

func (rc *realClock) NewTimer(d time.Duration) Timer {
	return realTimer{time.NewTimer(d)}
}

func (rc *realClock) AfterFunc(d time.Duration, f func()) Timer {
	return realTimer{time.AfterFunc(d, f)}
}

// FakeClock provides an interface for a clock which can be manually advanced
// through time.
//
// FakeClock maintains a list of "waiters," which consists of all callers
// waiting on the underlying clock (i.e. Tickers and Timers including callers of
// Sleep or After). Users can call BlockUntil to block until the clock has an
// expected number of waiters.
type FakeClock struct {
	// l protects all attributes of the clock, including all attributes of all
	// waiters and blockers.
	l        sync.RWMutex
	waiters  []expirer
	blockers []*blocker
	time     time.Time
}

// NewFakeClock returns a FakeClock implementation which can be
// manually advanced through time for testing. The initial time of the
// FakeClock will be the current system time.
//
// Tests that require a deterministic time must use NewFakeClockAt.
func NewFakeClock() *FakeClock {
	return NewFakeClockAt(time.Now())
}

// NewFakeClockAt returns a FakeClock initialised at the given time.Time.
func NewFakeClockAt(t time.Time) *FakeClock {
	return &FakeClock{
		time: t,
	}
}

// blocker is a caller of BlockUntil.
type blocker struct {
	count int

	// ch is closed when the underlying clock has the specified number of blockers.
	ch chan struct{}
}

// expirer is a timer or ticker that expires at some point in the future.
type expirer interface {
	// expire the expirer at the given time, returning the desired duration until
	// the next expiration, if any.
	expire(now time.Time) (next *time.Duration)

	// Get and set the expiration time.
	expiry() time.Time
	setExpiry(time.Time)
}

// After mimics [time.After]; it waits for the given duration to elapse on the
// fakeClock, then sends the current time on the returned channel.
func (fc *FakeClock) After(d time.Duration) <-chan time.Time {
	return fc.NewTimer(d).Chan()
}

// Sleep blocks until the given duration has passed on the fakeClock.
func (fc *FakeClock) Sleep(d time.Duration) {
	<-fc.After(d)
}

// Now returns the current time of the fakeClock
func (fc *FakeClock) Now() time.Time {
	fc.l.RLock()
	defer fc.l.RUnlock()
	return fc.time
}

// Since returns the duration that has passed since the given time on the
// fakeClock.
func (fc *FakeClock) Since(t time.Time) time.Duration {
	return fc.Now().Sub(t)
}

// Until returns the duration that has to pass from the given time on the fakeClock
// to reach the given time.
func (fc *FakeClock) Until(t time.Time) time.Duration {
	return t.Sub(fc.Now())
}

// NewTicker returns a Ticker that will expire only after calls to
// FakeClock.Advance() have moved the clock past the given duration.
//
// The duration d must be greater than zero; if not, NewTicker will panic.
func (fc *FakeClock) NewTicker(d time.Duration) Ticker {
	// Maintain parity with
	// https://cs.opensource.google/go/go/+/refs/tags/go1.20.3:src/time/tick.go;l=23-25
	if d <= 0 {
		panic(errors.New("non-positive interval for NewTicker"))
	}
	var ft *fakeTicker
	ft = &fakeTicker{
		firer: newFirer(),
		d:     d,
		reset: func(d time.Duration) { fc.set(ft, d) },
		stop:  func() { fc.stop(ft) },
	}
	fc.set(ft, d)
	return ft
}

// NewTimer returns a Timer that will fire only after calls to
// fakeClock.Advance() have moved the clock past the given duration.
func (fc *FakeClock) NewTimer(d time.Duration) Timer {
	return fc.newTimer(d, nil)
}

// AfterFunc mimics [time.AfterFunc]; it returns a Timer that will invoke the
// given function only after calls to fakeClock.Advance() have moved the clock
// past the given duration.
func (fc *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	return fc.newTimer(d, f)
}

// newTimer returns a new timer, using an optional afterFunc.
func (fc *FakeClock) newTimer(d time.Duration, afterfunc func()) *fakeTimer {
	var ft *fakeTimer
	ft = &fakeTimer{
		firer: newFirer(),
		reset: func(d time.Duration) bool {
			fc.l.Lock()
			defer fc.l.Unlock()
			// fc.l must be held across the calls to stopExpirer & setExpirer.
			stopped := fc.stopExpirer(ft)
			fc.setExpirer(ft, d)
			return stopped
		},
		stop: func() bool { return fc.stop(ft) },

		afterFunc: afterfunc,
	}
	fc.set(ft, d)
	return ft
}

// Advance advances fakeClock to a new point in time, ensuring waiters and
// blockers are notified appropriately before returning.
func (fc *FakeClock) Advance(d time.Duration) {
	fc.l.Lock()
	defer fc.l.Unlock()
	end := fc.time.Add(d)
	// Expire the earliest waiter until the earliest waiter's expiration is after
	// end.
	//
	// We don't iterate because the callback of the waiter might register a new
	// waiter, so the list of waiters might change as we execute this.
	for len(fc.waiters) > 0 && !end.Before(fc.waiters[0].expiry()) {
		w := fc.waiters[0]
		fc.waiters = fc.waiters[1:]

		// Use the waiter's expiration as the current time for this expiration.
		now := w.expiry()
		fc.time = now
		if d := w.expire(now); d != nil {
			// Set the new expiration if needed.
			fc.setExpirer(w, *d)
		}
	}
	fc.time = end
}

// BlockUntil blocks until the FakeClock has the given number of waiters.
//
// Prefer BlockUntilContext in new code, which offers context cancellation to
// prevent deadlock.
//
// Deprecated: New code should prefer BlockUntilContext.
func (fc *FakeClock) BlockUntil(n int) {
	fc.BlockUntilContext(context.TODO(), n)
}

// BlockUntilContext blocks until the fakeClock has the given number of waiters
// or the context is cancelled.
func (fc *FakeClock) BlockUntilContext(ctx context.Context, n int) error {
	b := fc.newBlocker(n)
	if b == nil {
		return nil
	}

	select {
	case <-b.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (fc *FakeClock) newBlocker(n int) *blocker {
	fc.l.Lock()
	defer fc.l.Unlock()
	// Fast path: we already have >= n waiters.
	if len(fc.waiters) >= n {
		return nil
	}
	// Set up a new blocker to wait for more waiters.
	b := &blocker{
		count: n,
		ch:    make(chan struct{}),
	}
	fc.blockers = append(fc.blockers, b)
	return b
}

// stop stops an expirer, returning true if the expirer was stopped.
func (fc *FakeClock) stop(e expirer) bool {
	fc.l.Lock()
	defer fc.l.Unlock()
	return fc.stopExpirer(e)
}

// stopExpirer stops an expirer, returning true if the expirer was stopped.
//
// The caller must hold fc.l.
func (fc *FakeClock) stopExpirer(e expirer) bool {
	idx := slices.Index(fc.waiters, e)
	if idx == -1 {
		return false
	}
	// Remove element, maintaining order, setting inaccessible elements to nil so
	// they can be garbage collected.
	copy(fc.waiters[idx:], fc.waiters[idx+1:])
	fc.waiters[len(fc.waiters)-1] = nil
	fc.waiters = fc.waiters[:len(fc.waiters)-1]
	return true
}

// set sets an expirer to expire at a future point in time.
func (fc *FakeClock) set(e expirer, d time.Duration) {
	fc.l.Lock()
	defer fc.l.Unlock()
	fc.setExpirer(e, d)
}

// setExpirer sets an expirer to expire at a future point in time.
//
// The caller must hold fc.l.
func (fc *FakeClock) setExpirer(e expirer, d time.Duration) {
	if d.Nanoseconds() <= 0 {
		// Special case for timers with duration <= 0: trigger immediately, never
		// reset.
		//
		// Tickers never get here, they panic if d is < 0.
		e.expire(fc.time)
		return
	}
	// Add the expirer to the set of waiters and notify any blockers.
	e.setExpiry(fc.time.Add(d))
	fc.waiters = append(fc.waiters, e)
	slices.SortFunc(fc.waiters, func(a, b expirer) int {
		return a.expiry().Compare(b.expiry())
	})

	// Notify blockers of our new waiter.
	var blocked []*blocker
	count := len(fc.waiters)
	for _, b := range fc.blockers {
		if b.count <= count {
			close(b.ch)
			continue
		}
		blocked = append(blocked, b)
	}
	fc.blockers = blocked
}

// firer is used by fakeTimer and fakeTicker used to help implement expirer.
type firer struct {
	// The channel associated with the firer, used to send expiration times.
	c chan time.Time

	// The time when the firer expires. Only meaningful if the firer is currently
	// one of a fakeClock's waiters.
	exp time.Time
}

func newFirer() firer {
	return firer{c: make(chan time.Time, 1)}
}

func (f *firer) Chan() <-chan time.Time {
	return f.c
}

// expiry implements expirer.
func (f *firer) expiry() time.Time {
	return f.exp
}

// setExpiry implements expirer.
func (f *firer) setExpiry(t time.Time) {
	f.exp = t
}
