package clockwork

import (
	"sort"
	"time"
)

// Timer provides an interface which can be used instead of directly
// using the timer within the time module. The real-time timer t
// provides events through t.C which becomes now t.Chan() to make
// this channel requirement definable in this interface.
type Timer interface {
	Chan() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
}

type realTimer struct {
	*time.Timer
}

func (r realTimer) Chan() <-chan time.Time {
	return r.C
}

type fakeTimer struct {
	// The channel associated with this timer. Only relevant for timers that
	// originate from NewTimer or similar, i.e. not for AfterFunc or other
	// internal usage.
	c chan time.Time

	// The fake clock driving events for this timer.
	clock *fakeClock

	// The time when the timer expires. Only meaningful if the timer is currently
	// one of the fake clock's sleepers.
	until time.Time

	// callback will get called synchronously with the lock of the clock being
	// held. It receives the time at which the timer expired.
	callback func(time.Time)
}

func (f *fakeTimer) Chan() <-chan time.Time {
	return f.c
}

func (f *fakeTimer) Stop() bool {
	f.clock.l.Lock()
	defer f.clock.l.Unlock()
	return f.stopImpl()
}

func (f *fakeTimer) stopImpl() bool {
	for i, t := range f.clock.sleepers {
		if t == f {
			// Remove element, maintaining order.
			copy(f.clock.sleepers[i:], f.clock.sleepers[i+1:])
			f.clock.sleepers[len(f.clock.sleepers)-1] = nil
			f.clock.sleepers = f.clock.sleepers[:len(f.clock.sleepers)-1]
			return true
		}
	}
	return false
}

func (f *fakeTimer) Reset(d time.Duration) bool {
	f.clock.l.Lock()
	defer f.clock.l.Unlock()
	stopped := f.stopImpl()
	f.resetImpl(d)
	return stopped
}

func (f *fakeTimer) resetImpl(d time.Duration) {
	now := f.clock.time
	if d.Nanoseconds() <= 0 {
		// special case - trigger immediately
		f.callback(now)
	} else {
		// otherwise, add to the set of sleepers
		f.until = f.clock.time.Add(d)
		f.clock.sleepers = append(f.clock.sleepers, f)
		sort.Sort(f.clock.sleepers)
		// and notify any blockers
		f.clock.notifyBlockers()
	}
}
