package clockwork

import (
	"testing"
	"time"
)

func TestFakeClockTimers(t *testing.T) {
	fc := &fakeClock{}

	zero := fc.NewTimer(0)

	if zero.Stop() {
		t.Errorf("zero timer could be stopped")
	}
	select {
	case <-zero.Chan():
	default:
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

	if one.Stop() {
		t.Errorf("triggered timer could be stopped")
	}
	select {
	case <-one.Chan():
	default:
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
	select {
	case <-one.Chan():
	default:
		t.Errorf("reset to zero timer didn't emit time")
	}
}

// withTimeout checks that the test finished executing within a certain time.
// If it runs over time, the test will be failed immediately.
// This is not an accurate timer, it's just used to fail deadlocking tests.
func withTimeout(t *testing.T, d time.Duration, fn func()) {
	step := make(chan struct{})
	go func() {
		step <- struct{}{}
		fn()
		step <- struct{}{}
	}()
	<-step // Wait for start
	select {
	case <-step: // Wait for finish
	case <-time.After(d):
		t.Fatalf("timed out")
	}
}

func TestBlockingOnTimers(t *testing.T) {
	withTimeout(t, 100*time.Millisecond, func() {
		fc := &fakeClock{}

		fc.NewTimer(0)
		fc.BlockUntil(0)

		one := fc.NewTimer(1)
		fc.BlockUntil(1)

		one.Stop()
		fc.BlockUntil(0)

		one.Reset(1)
		fc.BlockUntil(1)

		_ = fc.NewTimer(2)
		_ = fc.NewTimer(3)
		fc.BlockUntil(3)

		one.Stop()
		fc.BlockUntil(2)

		fc.Advance(3)
		fc.BlockUntil(0)
	})
}

func TestAdvancePastAfter(t *testing.T) {
	fc := &fakeClock{}

	start := fc.Now()
	one := fc.After(1)
	two := fc.After(2)
	six := fc.After(6)

	fc.Advance(1)
	if start.Add(1).Sub(<-one) > 0 {
		t.Errorf("timestamp is too early")
	}

	fc.Advance(5)
	if start.Add(2).Sub(<-two) > 0 {
		t.Errorf("timestamp is too early")
	}
	if start.Add(6).Sub(<-six) > 0 {
		t.Errorf("timestamp is too early")
	}
}
