package clockwork

import (
	"testing"
	"time"
)

func TestFakeClockTimerStop(t *testing.T) {
	fc := &fakeClock{}

	ft := fc.NewTimer(1)
	ft.Stop()
	select {
	case <-ft.Chan():
		t.Errorf("received unexpected tick!")
	default:
	}
}

func TestFakeClockTimers(t *testing.T) {
	fc := &fakeClock{}

	zero := fc.NewTimer(0)

	if zero.Stop() {
		t.Errorf("zero timer could be stopped")
	}

	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()

	select {
	case <-zero.Chan():
	case <-timeout.C:
		t.Errorf("zero timer didn't emit time")
	}

	one := fc.NewTimer(1)

	select {
	case <-one.Chan():
		t.Errorf("non-zero timer did emit time")
	default:
	}
	if !one.Stop() {
		t.Errorf("non-zero timer couldn't be stopped")
	}

	fc.Advance(5)

	select {
	case <-one.Chan():
		t.Errorf("stopped timer did emit time")
	default:
	}

	if one.Reset(1) {
		t.Errorf("resetting stopped timer didn't return false")
	}
	if !one.Reset(1) {
		t.Errorf("resetting active timer didn't return true")
	}

	fc.Advance(1)

	select {
	case <-time.After(500 * time.Millisecond):
	}

	if one.Stop() {
		t.Errorf("triggered timer could be stopped")
	}

	timeout2 := time.NewTimer(500 * time.Millisecond)
	defer timeout2.Stop()

	select {
	case <-one.Chan():
	case <-timeout2.C:
		t.Errorf("triggered timer didn't emit time")
	}

	fc.Advance(1)

	select {
	case <-one.Chan():
		t.Errorf("triggered timer emitted time more than once")
	default:
	}

	one.Reset(0)

	if one.Stop() {
		t.Errorf("reset to zero timer could be stopped")
	}

	timeout3 := time.NewTimer(500 * time.Millisecond)
	defer timeout3.Stop()

	select {
	case <-one.Chan():
	case <-timeout3.C:
		t.Errorf("reset to zero timer didn't emit time")
	}
}

func TestFakeClockTimer_Race(t *testing.T) {
	fc := NewFakeClock()

	timer := fc.NewTimer(1 * time.Millisecond)
	defer timer.Stop()

	fc.Advance(1 * time.Millisecond)

	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()

	select {
	case <-timer.Chan():
		// Pass
	case <-timeout.C:
		t.Fatalf("Timer didn't detect the clock advance!")
	}
}

func TestFakeClockTimer_Race2(t *testing.T) {
	fc := NewFakeClock()
	timer := fc.NewTimer(5 * time.Second)
	for i := 0; i < 100; i++ {
		fc.Advance(5 * time.Second)
		<-timer.Chan()
		timer.Reset(5 * time.Second)
	}
	timer.Stop()
}

func TestAfterFunc(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	start := fc.Now()
	c := make(chan time.Time)
	timer := fc.AfterFunc(5*time.Second, func() {
		c <- fc.Now()
	})

	expect := func(secs int) {
		select {
		case nt := <-c:
			if nt.Sub(start) != time.Second*time.Duration(secs) {
				t.Fatalf("expected call at exactly %d seconds, not %d", secs, nt.Sub(start))
			}
		case <-time.After(time.Second):
			t.Fatalf("expecting afterfunc to fire, gave up waiting")
		}
	}

	fc.Advance(time.Second * 5) // total: 5
	expect(5)

	fc.Advance(time.Second) // total: 6
	timer.Reset(0)          // should fire immediately
	expect(6)

	timer.Reset(time.Second)
	fc.Advance(time.Second) // total: 7
	expect(7)

	timer.Reset(time.Second)
	fc.Advance(time.Second * 10) // total: 17
	expect(17)                   // fake clock blows right past...
}
