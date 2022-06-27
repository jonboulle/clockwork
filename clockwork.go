package clockwork

import (
	"sync"
	"time"
)

// Clock provides an interface that packages can use instead of directly
// using the time module, so that chronology-related behavior can be tested
type Clock interface {
	After(d time.Duration) <-chan time.Time
	Sleep(d time.Duration)
	Now() time.Time
	Since(t time.Time) time.Duration
	NewTicker(d time.Duration) Ticker
	NewTimer(d time.Duration) Timer
	AfterFunc(d time.Duration, f func()) Timer
}

// FakeClock provides an interface for a clock which can be
// manually advanced through time
type FakeClock interface {
	Clock
	// Advance advances the FakeClock to a new point in time, ensuring any existing
	// sleepers are notified appropriately before returning
	Advance(d time.Duration)
	// BlockUntil will block until the FakeClock has the given number of
	// sleepers (callers of Sleep or After)
	BlockUntil(n int)
}

// NewRealClock returns a Clock which simply delegates calls to the actual time
// package; it should be used by packages in production.
func NewRealClock() Clock {
	return &realClock{}
}

// NewFakeClock returns a FakeClock implementation which can be
// manually advanced through time for testing. The initial time of the
// FakeClock will be an arbitrary non-zero time.
func NewFakeClock() FakeClock {
	// use a fixture that does not fulfill Time.IsZero()
	return NewFakeClockAt(time.Date(1984, time.April, 4, 0, 0, 0, 0, time.UTC))
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
	sleepers sleepers
	blockers []*blocker
	time     time.Time

	l sync.RWMutex
}

// blocker represents a caller of BlockUntil
type blocker struct {
	count int
	ch    chan struct{}
}

type sleepers []*fakeTimer

func (s sleepers) Len() int           { return len(s) }
func (s sleepers) Less(i, j int) bool { return s[i].until.Before(s[j].until) }
func (s sleepers) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// After mimics time.After; it waits for the given duration to elapse on the
// fakeClock, then sends the current time on the returned channel.
func (fc *fakeClock) After(d time.Duration) <-chan time.Time {
	return fc.NewTimer(d).Chan()
}

// notifyBlockers notifies all the blockers waiting until the at least the given
// number of sleepers are waiting on the fakeClock. It returns an updated slice
// of blockers (i.e. those still waiting)
func notifyBlockers(blockers []*blocker, count int) (newBlockers []*blocker) {
	for _, b := range blockers {
		if b.count <= count {
			close(b.ch)
		} else {
			newBlockers = append(newBlockers, b)
		}
	}
	return
}

// Sleep blocks until the given duration has passed on the fakeClock
func (fc *fakeClock) Sleep(d time.Duration) {
	<-fc.After(d)
}

// Now returns the current time of the fakeClock
func (fc *fakeClock) Now() time.Time {
	fc.l.RLock()
	t := fc.time
	fc.l.RUnlock()
	return t
}

// Since returns the duration that has passed since the given time on the fakeClock
func (fc *fakeClock) Since(t time.Time) time.Duration {
	return fc.Now().Sub(t)
}

// NewTicker returns a ticker that will expire only after calls to FakeClock
// Advance have moved the clock past the given duration.
func (fc *fakeClock) NewTicker(d time.Duration) Ticker {
	c := make(chan time.Time, 1)
	var ft *fakeTicker
	ft = &fakeTicker{
		fakeTimer: fakeTimer{
			c:     c,
			clock: fc,
			callback: func(now time.Time) {
				ft.resetImpl(d)
				select {
				case c <- now:
				default:
				}
			},
		},
	}
	ft.Reset(d)
	return ft
}

// NewTimer returns a timer that will fire only after calls to FakeClock Advance
// have moved the clock past the given duration.
func (fc *fakeClock) NewTimer(d time.Duration) Timer {
	c := make(chan time.Time, 1)
	ft := &fakeTimer{
		c:     c,
		clock: fc,
		callback: func(now time.Time) {
			select {
			case c <- now:
			default:
			}
		},
	}
	ft.Reset(d)
	return ft
}

// AfterFunc returns a timer that will invoke the given function only after
// calls to fakeClock Advance have moved the clock passed the given duration.
func (fc *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	ft := &fakeTimer{
		clock:    fc,
		callback: func(_ time.Time) { go f() },
	}
	ft.Reset(d)
	return ft
}

// Advance advances fakeClock to a new point in time, ensuring channels from any
// previous invocations of After are notified appropriately before returning
func (fc *fakeClock) Advance(d time.Duration) {
	fc.l.Lock()
	defer fc.l.Unlock()
	end := fc.time.Add(d)
	// While first sleeper is ready to wake, wake it. We don't iterate because the
	// callback of the sleeper might register a new sleeper, so the list of
	// sleepers might change as we execute this.
	for len(fc.sleepers) > 0 && !end.Before(fc.sleepers[0].until) {
		first := fc.sleepers[0]
		fc.sleepers = fc.sleepers[1:]
		fc.time = first.until
		first.callback(first.until)
	}
	fc.time = end
}

// BlockUntil will block until the fakeClock has the given number of sleepers
// (callers of Sleep or After)
func (fc *fakeClock) BlockUntil(n int) {
	fc.l.Lock()
	// Fast path: we already have >= n sleepers.
	if len(fc.sleepers) >= n {
		fc.l.Unlock()
		return
	}
	// Otherwise, we have < n sleepers. Set up a new blocker to wait for more.
	b := &blocker{
		count: n,
		ch:    make(chan struct{}),
	}
	fc.blockers = append(fc.blockers, b)
	fc.l.Unlock()
	<-b.ch
}
