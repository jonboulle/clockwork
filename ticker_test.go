package clockwork

import (
	"context"
	"testing"
	"time"
)

func TestFakeTickerStop(t *testing.T) {
	t.Parallel()
	fc := &FakeClock{}

	ft := fc.NewTicker(1)
	ft.Stop()
	select {
	case <-ft.Chan():
		t.Errorf("received unexpected tick!")
	default:
	}
}

func TestFakeTickerTick(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fc := &FakeClock{}
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
	case <-ctx.Done():
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
	case <-ctx.Done():
		t.Errorf("expected tick!")
	}
	ft.Stop()
}

func TestFakeTicker_Race(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fc := NewFakeClock()

	tickTime := 1 * time.Millisecond
	ticker := fc.NewTicker(tickTime)
	defer ticker.Stop()

	fc.Advance(tickTime)

	select {
	case <-ticker.Chan():
	case <-ctx.Done():
		t.Fatalf("Ticker didn't detect the clock advance!")
	}
}

func TestFakeTicker_Race2(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	fc := NewFakeClock()
	ft := fc.NewTicker(5 * time.Second)
	for i := 0; i < 100; i++ {
		fc.Advance(5 * time.Second)
		select {
		case <-ft.Chan():
		case <-ctx.Done():
			t.Fatalf("Ticker didn't detect the clock advance!")
		}

	}
	ft.Stop()
}

func TestFakeTicker_DeliveryOrder(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	fc := NewFakeClock()
	ticker := fc.NewTicker(2 * time.Second).Chan()
	timer := fc.NewTimer(5 * time.Second).Chan()
	go func() {
		for j := 0; j < 10; j++ {
			fc.BlockUntil(1)
			fc.Advance(1 * time.Second)
		}
	}()
	<-ticker
	a := <-timer
	// Only perform ordering check if ticker channel is drained at first.
	select {
	case <-ticker:
	default:
		select {
		case b := <-ticker:
			if a.After(b) {
				t.Fatalf("Expected timer before ticker, got timer %v after %v", a, b)
			}
		case <-ctx.Done():
			t.Fatalf("Expected ticker event didn't arrive!")
		}
	}
}
