package clockwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// Use a consistent timeout across tests that block on channels. Keeps test
// timeouts limited while being able to easily extend it to allow the test
// process to get killed, providing a stack trace.
const timeout = time.Minute

func TestFakeClockAfter(t *testing.T) {
	t.Parallel()
	fc := &fakeClock{}

	neg := fc.After(-1)
	select {
	case <-neg:
	default:
		t.Errorf("negative did not return!")
	}

	zero := fc.After(0)
	select {
	case <-zero:
	default:
		t.Errorf("zero did not return!")
	}
	one := fc.After(1)
	two := fc.After(2)
	six := fc.After(6)
	ten := fc.After(10)
	fc.Advance(1)
	select {
	case <-one:
	default:
		t.Errorf("one did not return!")
	}
	select {
	case <-two:
		t.Errorf("two returned prematurely!")
	case <-six:
		t.Errorf("six returned prematurely!")
	case <-ten:
		t.Errorf("ten returned prematurely!")
	default:
	}
	fc.Advance(1)
	select {
	case <-two:
	default:
		t.Errorf("two did not return!")
	}
	select {
	case <-six:
		t.Errorf("six returned prematurely!")
	case <-ten:
		t.Errorf("ten returned prematurely!")
	default:
	}
	fc.Advance(1)
	select {
	case <-six:
		t.Errorf("six returned prematurely!")
	case <-ten:
		t.Errorf("ten returned prematurely!")
	default:
	}
	fc.Advance(3)
	select {
	case <-six:
	default:
		t.Errorf("six did not return!")
	}
	select {
	case <-ten:
		t.Errorf("ten returned prematurely!")
	default:
	}
	fc.Advance(100)
	select {
	case <-ten:
	default:
		t.Errorf("ten did not return!")
	}
}

func TestNewFakeClock(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	now := fc.Now()
	if now.IsZero() {
		t.Fatalf("fakeClock.Now() fulfills IsZero")
	}

	now2 := fc.Now()
	if !reflect.DeepEqual(now, now2) {
		t.Fatalf("fakeClock.Now() returned different value: want=%#v got=%#v", now, now2)
	}
}

func TestNewFakeClockAt(t *testing.T) {
	t.Parallel()
	t1 := time.Date(1999, time.February, 3, 4, 5, 6, 7, time.UTC)
	fc := NewFakeClockAt(t1)
	now := fc.Now()
	if !reflect.DeepEqual(now, t1) {
		t.Fatalf("fakeClock.Now() returned unexpected non-initialised value: want=%#v, got %#v", t1, now)
	}
}

func TestFakeClockSince(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	now := fc.Now()
	elapsedTime := time.Second
	fc.Advance(elapsedTime)
	if fc.Since(now) != elapsedTime {
		t.Fatalf("fakeClock.Since() returned unexpected duration, got: %d, want: %d", fc.Since(now), elapsedTime)
	}
}

// This used to result in a deadlock.
// https://github.com/jonboulle/clockwork/issues/35
func TestTwoBlockersOneBlock(t *testing.T) {
	t.Parallel()
	fc := &fakeClock{}

	ft1 := fc.NewTicker(time.Second)
	ft2 := fc.NewTicker(time.Second)

	fc.BlockUntil(1)
	fc.BlockUntil(2)
	ft1.Stop()
	ft2.Stop()
}

func TestBlockUntilContext(t *testing.T) {
	t.Parallel()
	fc := &fakeClock{}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	blockCtx, cancelBlock := context.WithCancel(ctx)
	errCh := make(chan error)

	go func() {
		select {
		case errCh <- fc.BlockUntilContext(blockCtx, 2):
		case <-ctx.Done(): // Error case, captured below.
		}
	}()
	cancelBlock()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("BlockUntilContext returned %v, want context.Canceled.", err)
		}
	case <-ctx.Done():
		t.Errorf("Never receved error on context cancellation.")
	}
}

func TestAfterDeliveryInOrder(t *testing.T) {
	t.Parallel()
	fc := &fakeClock{}
	for i := 0; i < 1000; i++ {
		three := fc.After(3 * time.Second)
		for j := 0; j < 100; j++ {
			fc.After(1 * time.Second)
		}
		two := fc.After(2 * time.Second)
		go func() {
			fc.Advance(5 * time.Second)
		}()
		<-three
		select {
		case <-two:
		default:
			t.Fatalf("Signals from After delivered out of order")
		}
	}
}

// TestFakeClockRace detects data races in fakeClock when invoked with run using `go -race ...`.
// There are no failure conditions when invoked without the -race flag.
func TestFakeClockRace(t *testing.T) {
	t.Parallel()
	fc := &fakeClock{}
	d := time.Second
	go func() { fc.Advance(d) }()
	go func() { fc.NewTicker(d) }()
	go func() { fc.NewTimer(d) }()
	go func() { fc.Sleep(d) }()
}

func TestExpirations(t *testing.T) {
	t.Parallel()

	t.Run("AfterFunc increments counter", func(t *testing.T) {
		fc := &fakeClock{}
		fc.AfterFunc(time.Minute, func() {})
		fc.AfterFunc(2*time.Minute, func() {})
		fc.AfterFunc(2*time.Minute, func() {})
		fc.AfterFunc(3*time.Minute, func() {})

		start := fc.Now()

		fc.Advance(time.Minute)
		want := 1
		if got := fc.Expirations(); got != want {
			t.Errorf("after %v, fc.Expirations() = %v, want %v", fc.Since(start), got, want)
		}

		fc.Advance(time.Minute)
		want = 3
		if got := fc.Expirations(); got != want {
			t.Errorf("after %v, fc.Expirations() = %v, want %v", fc.Since(start), got, want)
		}

		fc.Advance(30 * time.Second) // should not cause expirations.
		if got := fc.Expirations(); got != want {
			t.Errorf("after %v, fc.Expirations() = %v, want %v", fc.Since(start), got, want)
		}
	})

	t.Run("Calls to Stop do not increment counter", func(t *testing.T) {
		fc := &fakeClock{}
		ticker := fc.NewTicker(time.Minute)
		timer := fc.NewTimer(time.Minute)

		start := fc.Now()

		// Advance a little for good measure, but should have no effect.
		fc.Advance(30 * time.Second)
		want := 0
		if got := fc.Expirations(); got != want {
			t.Errorf("after %v, fc.Expirations() = %v, want %v", fc.Since(start), got, want)
		}

		timer.Stop()
		ticker.Stop()
		fc.Advance(time.Minute) // advances past the set expirations
		if got := fc.Expirations(); got != want {
			t.Errorf("after %v, fc.Expirations() = %v, want %v", fc.Since(start), got, want)
		}
	})

	t.Run("Dropped ticks increment counter", func(t *testing.T) {
		fc := &fakeClock{}
		ticker := fc.NewTicker(time.Minute)

		fc.Advance(2 * time.Minute)
		want := 2
		if got := fc.Expirations(); got != want {
			t.Errorf("fc.Expirations() = %v, want %v", got, want)
		}
		// As of this writing I am using a variable for ticker because I don't know
		// if assigning it to _ makes it eligible for garbage collection.
		//
		// Since we have to use the variable to appease the compiler, make sure we
		// can receive on the ticker channel.
		select {
		case <-ticker.Chan(): //
		default:
			t.Errorf("Ticker should have fired at least once.")
		}

	})
}
