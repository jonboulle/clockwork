package clockwork

import "time"

// Timer provides an interface which can be used instead of directly using
// [time.Timer]. The real-time timer t provides events through t.C which becomes
// t.Chan() to make this channel requirement definable in this interface.
type Timer interface {
	Chan() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
}

type realTimer struct{ *time.Timer }

func (r realTimer) Chan() <-chan time.Time {
	return r.C
}

type fakeTimer struct {
	firer

	// reset and stop provide the implementation of the respective exported
	// functions.
	reset func(d time.Duration) bool
	stop  func() bool

	// If present when the timer fires, the timer calls afterFunc in its own
	// goroutine rather than sending the time on Chan().
	afterFunc func()
}

func newFakeTimer(fc *FakeClock, afterfunc func()) *fakeTimer {
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
	return ft
}

func (f *fakeTimer) Reset(d time.Duration) bool {
	return f.reset(d)
}

func (f *fakeTimer) Stop() bool {
	return f.stop()
}

func (f *fakeTimer) expire(now time.Time) *time.Duration {
	if f.afterFunc != nil {
		go f.afterFunc()
		return nil
	}

	// Never block on expiration.
	select {
	case f.c <- now:
	default:
	}
	return nil
}
