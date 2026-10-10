package clockwork

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestManualCancelCompletesBeforeReturning(t *testing.T) {
	// Keep the cancellation goroutine from relying on a second processor to
	// finish before the caller checks Done. This test is deliberately not parallel.
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	for _, deadline := range []bool{false, true} {
		clock := NewFakeClock()
		var ctx context.Context
		var cancel context.CancelFunc
		if deadline {
			ctx, cancel = WithDeadline(context.Background(), clock, clock.Now().Add(time.Hour))
		} else {
			ctx, cancel = WithTimeout(context.Background(), clock, time.Hour)
		}
		cancel()
		select {
		case <-ctx.Done():
		default:
			t.Errorf("deadline=%v: cancel returned while Done was open", deadline)
		}
		<-ctx.Done() // Also clean up the original implementation after a failure.
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("unexpected cancellation error: %v", ctx.Err())
		}
		cancel() // Repeated calls remain safe and complete.
	}
}

func TestConcurrentManualCancellationCompletes(t *testing.T) {
	clock := NewFakeClock()
	ctx, cancel := WithTimeout(context.Background(), clock, time.Hour)
	start := make(chan struct{})
	returnedEarly := make(chan struct{}, 32)
	var wg sync.WaitGroup
	for i := 0; i < cap(returnedEarly); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cancel()
			select {
			case <-ctx.Done():
			default:
				returnedEarly <- struct{}{}
			}
		}()
	}
	close(start)
	wg.Wait()
	<-ctx.Done()
	if len(returnedEarly) != 0 {
		t.Errorf("%d cancel calls returned before Done closed", len(returnedEarly))
	}
}

func TestCancelPreservesCompletedContextError(t *testing.T) {
	clock := NewFakeClock()
	ctx, cancel := WithTimeout(context.Background(), clock, time.Hour)
	clock.Advance(time.Hour)
	<-ctx.Done()
	cancel()
	if !errors.Is(ctx.Err(), ErrFakeClockDeadlineExceeded) {
		t.Fatalf("cancel replaced completed deadline error: %v", ctx.Err())
	}

	parent, cancelParent := context.WithCancel(context.Background())
	child, cancelChild := WithTimeout(parent, clock, time.Hour)
	cancelParent()
	<-child.Done()
	cancelChild()
	if !errors.Is(child.Err(), context.Canceled) {
		t.Fatalf("unexpected parent cancellation error: %v", child.Err())
	}

	realContext, realCancel := WithTimeout(context.Background(), NewRealClock(), time.Hour)
	realCancel()
	select {
	case <-realContext.Done():
	default:
		t.Fatal("real-clock cancellation changed")
	}
}
