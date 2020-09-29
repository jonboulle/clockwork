package clockwork

import (
	"testing"
	"time"
)

func TestFakeTickerStop(t *testing.T) {
	fc := &fakeClock{}

	ft := fc.NewTicker(1)
	ft.Stop()
	select {
	case <-ft.Chan():
		t.Errorf("received unexpected tick!")
	default:
	}
}

func TestFakeTickerTick(t *testing.T) {
	fc := &fakeClock{}
	now := fc.Now()

	// The tick at now.Add(2) should not get through since we advance time by
	// two units below and the channel can hold at most one tick until it's
	// consumed.
	first := now.Add(1)
	second := now.Add(3)

	// We wrap the Advance() calls with blockers to make sure that the ticker
	// can go to sleep and produce ticks without time passing in parallel.
	ft := fc.NewTicker(1)
	fc.BlockUntil(1)
	fc.Advance(2)
	fc.BlockUntil(1)

	select {
	case tick := <-ft.Chan():
		if tick != first {
			t.Errorf("wrong tick time, got: %v, want: %v", tick, first)
		}
	case <-time.After(time.Millisecond):
		t.Errorf("expected tick!")
	}

	// Advance by one more unit, we should get another tick now.
	fc.Advance(1)
	fc.BlockUntil(1)

	select {
	case tick := <-ft.Chan():
		if tick != second {
			t.Errorf("wrong tick time, got: %v, want: %v", tick, second)
		}
	case <-time.After(time.Millisecond):
		t.Errorf("expected tick!")
	}
	ft.Stop()
}

func TestFakeTicker_Race(t *testing.T) {
	fc := NewFakeClock()

	tickTime := 1 * time.Millisecond
	ticker := fc.NewTicker(tickTime)
	defer ticker.Stop()

	fc.Advance(tickTime)

	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()

	select {
	case <-ticker.Chan():
		// Pass
	case <-timeout.C:
		t.Fatalf("Ticker didn't detect the clock advance!")
	}
}

func TestFakeTicker_Race2(t *testing.T) {
	fc := NewFakeClock()
	ft := fc.NewTicker(5 * time.Second)
	for i := 0; i < 100; i++ {
		fc.Advance(5 * time.Second)
		<-ft.Chan()
	}
	ft.Stop()
}
