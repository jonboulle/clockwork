package clockwork

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// myFunc is an example of a time-dependent function, using an injected clock.
func myFunc(clock Clock, i *int) {
	clock.Sleep(3 * time.Second)
	*i += 1
}

// assertState is an example of a state assertion in a test.
func assertState(t *testing.T, i, j int) {
	if i != j {
		t.Fatalf("i %d, j %d", i, j)
	}
}

// TestMyFunc tests myFunc's behaviour with a FakeClock.
func TestMyFunc(t *testing.T) {
	ctx := context.Background()
	var i int
	c := NewFakeClock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		myFunc(c, &i)
		wg.Done()
	}()

	// Ensure we wait until myFunc is waiting on the clock.
	// Use a context to avoid blocking forever if something
	// goes wrong.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.BlockUntilContext(ctx, 1)

	// Assert the initial state.
	assertState(t, i, 0)

	// Now advance the clock forward in time.
	c.Advance(1 * time.Hour)

	// Wait until the function completes.
	wg.Wait()

	// Assert the final state.
	assertState(t, i, 1)
}

// myLoop is an example of a function that works periodically, using an
// injected clock, until ctx is done.
func myLoop(ctx context.Context, clock Clock, period time.Duration, i *atomic.Int32) {
	for {
		select {
		case <-clock.After(period):
		case <-ctx.Done():
			return
		}
		// The work takes some real time.
		time.Sleep(time.Millisecond)
		i.Add(1)
	}
}

// TestMyLoop tests two myLoop with a FakeClock.
func TestMyLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var fast, slow atomic.Int32
	c := NewFakeClock()

	go myLoop(ctx, c, 1*time.Second, &fast)
	go myLoop(ctx, c, 3*time.Second, &slow)

	// Ensure we wait until both loops are waiting on the clock.
	c.BlockUntilContext(ctx, 2)

	// Advance the clock, and wait until each loop it woke up waits on the
	// clock again.
	c.AdvanceAndBlockUntilRearmed(ctx, 1*time.Second)
	assertState(t, int(fast.Load()), 1)
	assertState(t, int(slow.Load()), 0)

	c.AdvanceAndBlockUntilRearmed(ctx, 1*time.Second)
	assertState(t, int(fast.Load()), 2)
	assertState(t, int(slow.Load()), 0)

	// Both loops wake up at 3 seconds.
	c.AdvanceAndBlockUntilRearmed(ctx, 1*time.Second)
	assertState(t, int(fast.Load()), 3)
	assertState(t, int(slow.Load()), 1)

	// A loop runs at most once per advance, since it waits on the clock again
	// after the advance.
	c.AdvanceAndBlockUntilRearmed(ctx, 3*time.Second)
	assertState(t, int(fast.Load()), 4)
	assertState(t, int(slow.Load()), 2)
}
