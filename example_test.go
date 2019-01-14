package clockwork

import (
	"sync"
	"testing"
	"time"
)

// myFunc is an example of a time-dependent function, using an
// injected clock
func myFunc(clock Clock, i *int) {
	clock.Sleep(3 * time.Second)
	*i += 1
}

// assertState is an example of a state assertion in a test
func assertState(t *testing.T, i, j int) {
	if i != j {
		t.Fatalf("i %d, j %d", i, j)
	}
}

// TestMyFunc tests myFunc's behaviour with a FakeClock
func TestMyFunc(t *testing.T) {
	var i int
	c := NewFakeClock()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		myFunc(c, &i)
		wg.Done()
	}()

	// Wait until myFunc is actually sleeping on the clock
	c.BlockUntil(1)

	// Assert the initial state
	assertState(t, i, 0)

	// Now advance the clock forward in time
	c.Advance(1 * time.Hour)

	// Wait until the function completes
	wg.Wait()

	// Assert the final state
	assertState(t, i, 1)
}
