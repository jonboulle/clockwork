package clockwork

import (
	"context"
	"sync"
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
