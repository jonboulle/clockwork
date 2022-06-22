package clockwork

import (
	"sync/atomic"
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
	c     chan time.Time
	clock *fakeClock

	// Timer is running if the LSB is 1. Also, generation number will change on
	// every reset, to help distinguish latest event from past registered events.
	generation uint32
}

func (f *fakeTimer) Chan() <-chan time.Time {
	return f.c
}

func (f *fakeTimer) Reset(d time.Duration) bool {
	if d <= 0 {
		stopped := f.Stop()
		f.send(f.clock.Now())
		return stopped
	}
	var pastGen, generation uint32
	for {
		pastGen = atomic.LoadUint32(&f.generation)
		// generation is the next odd number after pastGen
		generation = pastGen + 1 + (pastGen & 1)
		if atomic.CompareAndSwapUint32(&f.generation, pastGen, generation) {
			break
		}
	}
	// It would be better if we could use AfterFunc here, so that we don't leak
	// goroutines for events that never get consumed.
	after := f.clock.After(d)
	go func() {
		now := <-after
		if atomic.CompareAndSwapUint32(&f.generation, generation, generation+1) {
			// This is still the active timer, so send the time. There is a potential
			// race here, where another thread could reset the timer and get it to
			// expire before we manage to send here. Avoiding that would likely
			// require the use of a mutex in many places.
			f.send(now)
		}
	}()
	return pastGen&1 != 0
}

func (f *fakeTimer) Stop() bool {
	generation := atomic.LoadUint32(&f.generation)
	for generation&1 != 0 { // While currently running.
		if atomic.CompareAndSwapUint32(&f.generation, generation, generation+1) {
			// We changed from running to stopped state.
			return true
		}
		// Something else changed the value, check again.
		generation = atomic.LoadUint32(&f.generation)
	}
	// We merely observed the value in stopped state.
	return false
}

// Events are discarded if the underlying ticker channel does not have enough
// capacity.
func (f *fakeTimer) send(t time.Time) {
	select {
	case f.c <- t:
	default:
	}
}
