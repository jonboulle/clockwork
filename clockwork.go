package clockwork

import (
	"errors"
	"sort"
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
	NewTicker(d time.Duration) Ticker
	NewTimer(d time.Duration) Timer
	AfterFunc(d time.Duration, f func()) Timer
}

// FakeClock provides an interface for a clock which can be manually advanced
// through time.
//
// FakeClock maintains a list of "waiters," which consists of all Timers,
// Tickers, or callers of Sleep or After which use the underlying clock. Users
// can use BlockUntil to block until the clock has an expected number of
// waiters.
type FakeClock interface {
	Clock
	// Advance advances the FakeClock to a new point in time, ensuring any existing
	// waiters are notified appropriately before returning.
	Advance(d time.Duration)
	// BlockUntil will block until the FakeClock has the given number of waiters.
	BlockUntil(n int)
}

// NewRealClock returns a Clock which simply delegates calls to the actual
// [time] package; it should be used by packages in production.
func NewRealClock() Clock {
	return &realClock{}
}

// NewFakeClock returns a FakeClock implementation which can be
// manually advanced through time for testing. The initial time of the
// FakeClock will be an arbitrary non-zero time.
func NewFakeClock() FakeClock {
	// Use the standard layout time to avoid fulfilling Time.IsZero().
	return NewFakeClockAt(time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC))
}

// NewFakeClockAt returns a FakeClock initialised at the given time.Time.
func NewFakeClockAt(t time.Time) FakeClock {
	return &fakeClock{
		time: t,
	}
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

func (rc *realClock) NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

func (rc *realClock) NewTimer(d time.Duration) Timer {
	return realTimer{time.NewTimer(d)}
}

func (rc *realClock) AfterFunc(d time.Duration, f func()) Timer {
	return realTimer{time.AfterFunc(d, f)}
}

type fakeClock struct {
	// mu protects all attributes of the clock, including all attributes of all
	// waiters and blockers.
	mu       sync.RWMutex
	waiters  []expirer
	blockers []*blocker
	time     time.Time
}

// blocker represents a caller of BlockUntil
type blocker struct {
	count int
	ch    chan struct{}
}

// expirer is a timer or ticker that expires at some point in the future.
type expirer interface {
	// expire the expirer at the given time, returning duration until the next
	// expiration, if any.
	expire(now time.Time) (next *time.Duration)

	// Get and set the expiration time.
	expiry() time.Time
	setExpiry(time.Time)
}

// After mimics [time.After]; it waits for the given duration to elapse on the
// fakeClock, then sends the current time on the returned channel.
func (fc *fakeClock) After(d time.Duration) <-chan time.Time {
	return fc.NewTimer(d).Chan()
}

// Sleep blocks until the given duration has passed on the fakeClock.
func (fc *fakeClock) Sleep(d time.Duration) {
	<-fc.After(d)
}

// Now returns the current time of the fakeClock
func (fc *fakeClock) Now() time.Time {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.time
}

// Since returns the duration that has passed since the given time on the
// fakeClock.
func (fc *fakeClock) Since(t time.Time) time.Duration {
	return fc.Now().Sub(t)
}

// NewTicker returns a ticker that will expire only after calls to
// FakeClock.Advance have moved the clock past the given duration.
func (fc *fakeClock) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		panic(errors.New("non-positive interval for NewTicker"))
	}
	var ft *fakeTicker
	ft = &fakeTicker{
		firer: newFirer(),
		d:     d,
		reset: func(d time.Duration) { fc.lockedSet(ft, d) },
		stop:  func() { fc.lockedStop(ft) },
	}
	fc.lockedSet(ft, d)
	return ft
}

// NewTimer returns a Timer that will fire only after calls to fakeClock.Advance
// have moved the clock past the given duration.
func (fc *fakeClock) NewTimer(d time.Duration) Timer {
	return fc.newTimer(d, nil)
}

// AfterFunc returns a Timer that will invoke the given function only after
// calls to FakeClock.Advance have moved the clock passed the given duration.
func (fc *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	return fc.newTimer(d, f)
}

// newTimer returns a new timer, using an optional afterFunc.
func (fc *fakeClock) newTimer(d time.Duration, afterfunc func()) *fakeTimer {
	var ft *fakeTimer
	ft = &fakeTimer{
		firer: newFirer(),
		reset: func(d time.Duration) bool {
			fc.mu.Lock()
			defer fc.mu.Unlock()
			stopped := fc.stop(ft)
			fc.set(ft, d)
			return stopped
		},
		stop: func() bool { return fc.lockedStop(ft) },

		afterFunc: afterfunc,
	}
	fc.lockedSet(ft, d)
	return ft
}

// Advance advances fakeClock to a new point in time, ensuring channels from any
// previous invocations of After are notified appropriately before returning
func (fc *fakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	end := fc.time.Add(d)
	// While first sleeper is ready to wake, wake it. We don't iterate because the
	// callback of the sleeper might register a new sleeper, so the list of
	// sleepers might change as we execute this.
	for len(fc.waiters) > 0 && !end.Before(fc.waiters[0].expiry()) {
		first := fc.waiters[0]
		fc.waiters = fc.waiters[1:]
		fc.time = first.expiry()
		if d := first.expire(first.expiry()); d != nil {
			fc.set(first, *d)
		}
	}
	fc.time = end
}

// BlockUntil will block until the fakeClock has the given number of sleepers
// (callers of Sleep or After)
func (fc *fakeClock) BlockUntil(n int) {
	fc.mu.Lock()
	// Fast path: we already have >= n sleepers.
	if len(fc.waiters) >= n {
		fc.mu.Unlock()
		return
	}
	// Otherwise, we have < n sleepers. Set up a new blocker to wait for more.
	b := &blocker{
		count: n,
		ch:    make(chan struct{}),
	}
	fc.blockers = append(fc.blockers, b)
	fc.mu.Unlock()
	<-b.ch
}

func (fc *fakeClock) lockedStop(f expirer) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.stop(f)
}

// Stop an expirer.
//
// The caller must hold fc.mu.
func (fc *fakeClock) stop(f expirer) bool {
	for i, t := range fc.waiters {
		if t == f {
			// Remove element, maintaining order.
			copy(fc.waiters[i:], fc.waiters[i+1:])
			fc.waiters[len(fc.waiters)-1] = nil
			fc.waiters = fc.waiters[:len(fc.waiters)-1]
			return true
		}
	}
	return false
}

func (fc *fakeClock) lockedSet(f expirer, d time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.set(f, d)
}

// Set an expirer to expire at a future point in time.
//
// The caller must hold fc.mu.
func (fc *fakeClock) set(f expirer, d time.Duration) {
	if d.Nanoseconds() <= 0 {
		// special case - trigger immediately, never reset
		f.expire(fc.time)
		return
	}
	// Add to the set of sleepers and notify any blockers.
	f.setExpiry(fc.time.Add(d))
	fc.waiters = append(fc.waiters, f)
	sort.Slice(fc.waiters, func(i int, j int) bool {
		return fc.waiters[i].expiry().Before(fc.waiters[j].expiry())
	})
	fc.notifyBlockers()
}

// notifyBlockers notifies all the blockers waiting for the current number of
// sleepers or fewer.
func (fc *fakeClock) notifyBlockers() {
	var stillWaiting []*blocker
	count := len(fc.waiters)
	for _, b := range fc.blockers {
		if b.count <= count {
			close(b.ch)
			continue
		}
		stillWaiting = append(stillWaiting, b)
	}
	fc.blockers = stillWaiting
}

// firer is used in timer and ticker used to help implement expirer.
type firer struct {
	// The channel associated with the firer.
	c chan time.Time

	// The time when the firer expires. Only meaningful if the firer is currently
	// one of a fake clock's waiters.
	exp time.Time
}

func newFirer() firer {
	return firer{c: make(chan time.Time, 1)}
}

func (f *firer) Chan() <-chan time.Time {
	return f.c
}

func (f *firer) expiry() time.Time {
	return f.exp
}

func (f *firer) setExpiry(t time.Time) {
	f.exp = t
}
