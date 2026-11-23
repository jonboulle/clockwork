package clockwork

import (
	"context"
	"testing"
	"time"
)

func TestContextCancellationStopsTimer(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		for _, parentCancellation := range []bool{false, true} {
			clock := NewFakeClock()
			parent, cancelParent := context.WithCancel(context.Background())
			var ctx context.Context
			var cancel context.CancelFunc
			if deadline {
				ctx, cancel = WithDeadline(parent, clock, clock.Now().Add(time.Hour))
			} else {
				ctx, cancel = WithTimeout(parent, clock, time.Hour)
			}
			if parentCancellation {
				cancelParent()
			} else {
				cancel()
			}
			select {
			case <-ctx.Done():
			case <-time.After(timeout):
				t.Fatal("context was not canceled")
			}
			cancel()
			cancelParent()

			// A completed context must not count as a pending fake-clock waiter.
			probe, cancelProbe := context.WithCancel(context.Background())
			cancelProbe()
			if err := clock.BlockUntilContext(probe, 1); err != context.Canceled {
				t.Errorf("deadline=%v, parentCancellation=%v: completed context still has a timer", deadline, parentCancellation)
			}

			// Timers belonging to other callers still behave normally.
			timer := clock.NewTimer(time.Second)
			clock.Advance(time.Second)
			select {
			case <-timer.Chan():
			default:
				t.Error("independent timer did not fire")
			}
		}
	}
}
