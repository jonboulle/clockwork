package clockwork

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Use a consistent timeout across tests that block on channels. Keeps test
// timeouts limited while being able to easily extend it to allow the test
// process to get killed, providing a stack trace.
const timeout = time.Minute

func TestAfter(t *testing.T) {
	t.Parallel()
	fc := &FakeClock{}

	var timers []<-chan time.Time
	for i := 0; i < 3; i++ {
		timers = append(timers, fc.After(time.Duration(i*2+1))) // 1, 3, 5
	}

	// Nothing fired immediately.
	for i, ch := range timers {
		select {
		case <-ch:
			t.Errorf("Timer at time=%v fired at time=0", i*2+1)
		default:
		}
	}

	// First timer fires at time=1.
	fc.Advance(1)
	select {
	case <-timers[0]:
	default:
		t.Errorf("Timer at time=1 did not fire at time=1")
	}
	for i, ch := range timers[1:] {
		select {
		case <-ch:
			t.Errorf("Timer at time=%v fired at time=1", i*2+3)
		default:
		}
	}

	// Should not change anything.
	fc.Advance(1)
	for i, ch := range timers[1:] {
		select {
		case <-ch:
			t.Errorf("Timer at time=%v fired at time=2", i*2+3)
		default:
		}
	}

	// Add 1 more timer at time 5. Should fire at the same time as our timer in chs[2]
	timers = append(timers, fc.After(time.Duration(3))) // Current time + 3 = 2 + 3 = 5

	// Skip over timer at time 3, advancing directly to 4. Check it works as expected.
	fc.Advance(2)
	select {
	case <-timers[1]:
	default:
		t.Errorf("Timer at time=3 did not fire at time=4")
	}
	for _, i := range []int{2, 3} {
		select {
		case <-timers[i]:
			t.Errorf("Timer at time=5 fired at time=4")
		default:
		}
	}

	fc.Advance(1)
	for idx, tIdex := range []int{2, 3} {
		select {
		case <-timers[tIdex]:
		default:
			t.Errorf("Timer at time=5 #%v did not fire at time=5", idx)
		}
	}
}

func TestAfterZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string

		d time.Duration
	}{
		{name: "zero"},
		{
			name: "negative",
			d:    -time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &FakeClock{}
			select {
			case <-fc.After(tc.d):
			case <-ctx.Done():
				t.Errorf("FakeClock.After() did not return.")
			}
		})
	}
}

func TestNewFakeClockIsNotZero(t *testing.T) {
	t.Parallel()
	fc := NewFakeClock()
	if fc.Now().IsZero() {
		t.Errorf("NewFakeClock.Now().IsZero() returned true, want false")
	}
}

func TestNewFakeClockAt(t *testing.T) {
	t.Parallel()
	want := time.Date(1999, time.February, 3, 4, 5, 6, 7, time.UTC)
	if got := NewFakeClockAt(want).Now(); !got.Equal(want) {
		t.Errorf("fakeClock.Now() returned %v, want: %v", got, want)
	}
}

func TestSince(t *testing.T) {
	t.Parallel()
	start := time.Date(1999, time.February, 3, 4, 5, 6, 7, time.UTC)
	want := time.Second
	fc := NewFakeClockAt(start.Add(want))
	if got := fc.Since(start); got != want {
		t.Errorf("fakeClock.Since() returned %v, want: %v", got, want)
	}
}

func TestUntil(t *testing.T) {
	t.Parallel()
	start := time.Date(1999, time.February, 3, 4, 5, 6, 7, time.UTC)
	fc := NewFakeClockAt(start)
	want := time.Second
	end := start.Add(want)
	if got := fc.Until(end); got != want {
		t.Errorf("fakeClock.Until() returned %v, want: %v", got, want)
	}
}

func TestBlockUntilContext(t *testing.T) {
	t.Parallel()
	fc := &FakeClock{}

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
		t.Errorf("Never received error on context cancellation.")
	}
}

func TestAfterDeliveryInOrder(t *testing.T) {
	t.Parallel()
	fc := &FakeClock{}
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
	fc := &FakeClock{}
	d := time.Second
	go func() { fc.Advance(d) }()
	go func() { fc.NewTicker(d) }()
	go func() { fc.NewTimer(d) }()
	go func() { fc.Sleep(d) }()
}
