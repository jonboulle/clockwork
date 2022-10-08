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

func TestFakeClockTimer_ResetRace(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	d := 5 * time.Second
	var times []time.Time
	timer := fc.NewTimer(d)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				break
			case now := <-timer.Chan():
				times = append(times, now)
			}
		}
	}()
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			timer.Reset(d)
		}
		fc.Advance(d)
	}
	timer.Stop()
	close(done)
	for i := 1; i < len(times); i++ {
		if times[i-1] == times[i] {
			t.Fatalf("Timer repeatedly reported the same time.")
		}
	}
}

func TestFakeClockTimer_ZeroResetDoesNotBlock(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	timer := fc.NewTimer(0)
	for i := 0; i < 10; i++ {
		timer.Reset(0)
	}
	<-timer.Chan()
}

func TestAfterFunc_Concurrent(t *testing.T) {
	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()

	fc := NewFakeClock()
	blocker := make(chan struct{})
	ch := make(chan int)
	// AfterFunc should start goroutines, so each should be able to make progress
	// independent of the others.
	fc.AfterFunc(2*time.Second, func() {
		<-blocker
		ch <- 222
	})
	fc.AfterFunc(2*time.Second, func() {
		ch <- 111
	})
	fc.AfterFunc(2*time.Second, func() {
		<-blocker
		ch <- 222
	})
	fc.Advance(2 * time.Second)
	select {
	case a := <-ch:
		if a != 111 {
			t.Fatalf("Expected 111, got %d", a)
		}
	case <-timeout.C:
		t.Fatalf("Expected signal hasn't arrived")
	}
	close(blocker)
	select {
	case a := <-ch:
		if a != 222 {
			t.Fatalf("Expected 222, got %d", a)
		}
	case <-timeout.C:
		t.Fatalf("Expected signal hasn't arrived")
	}
	select {
	case a := <-ch:
		if a != 222 {
			t.Fatalf("Expected 222, got %d", a)
		}
	case <-timeout.C:
		t.Fatalf("Expected signal hasn't arrived")
	}
}
