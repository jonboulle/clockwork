package clockwork

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFakeClockContextErr(t *testing.T) {
	t.Parallel()
	factories := []struct {
		name string
		new  func(context.Context, *FakeClock) (context.Context, context.CancelFunc)
	}{
		{"WithTimeout", func(parent context.Context, clock *FakeClock) (context.Context, context.CancelFunc) {
			return WithTimeout(parent, clock, time.Hour)
		}},
		{"WithDeadline", func(parent context.Context, clock *FakeClock) (context.Context, context.CancelFunc) {
			return WithDeadline(parent, clock, clock.Now().Add(time.Hour))
		}},
	}
	actions := []struct {
		name string
		do   func(context.CancelFunc, context.CancelFunc, *FakeClock)
		want error
	}{
		{"cancel", func(cancel, _ context.CancelFunc, _ *FakeClock) { cancel() }, context.Canceled},
		{"parent cancel", func(_, cancelParent context.CancelFunc, _ *FakeClock) { cancelParent() }, context.Canceled},
		{"deadline", func(_, _ context.CancelFunc, clock *FakeClock) { clock.Advance(time.Hour) }, ErrFakeClockDeadlineExceeded},
	}
	for _, factory := range factories {
		factory := factory
		for _, action := range actions {
			action := action
			t.Run(factory.name+"/"+action.name, func(t *testing.T) {
				t.Parallel()
				parent, cancelParent := context.WithCancel(context.Background())
				defer cancelParent()
				clock := NewFakeClockAt(time.Unix(0, 0))
				ctx, cancel := factory.new(parent, clock)
				defer cancel()

				result := make(chan error, 1)
				go func() { result <- ctx.Err() }()
				select {
				case err := <-result:
					if err != nil {
						t.Fatalf("active context Err() = %v, want nil", err)
					}
				case <-time.After(timeout):
					t.Fatal("Err blocked before the context was canceled")
				}

				action.do(cancel, cancelParent, clock)
				select {
				case <-ctx.Done():
				case <-time.After(timeout):
					t.Fatal("context was not canceled")
				}
				for i := 0; i < 2; i++ {
					if err := ctx.Err(); err != action.want {
						t.Errorf("canceled context Err() = %v, want %v", err, action.want)
					}
				}
				if action.want == ErrFakeClockDeadlineExceeded && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					t.Error("deadline error does not wrap context.DeadlineExceeded")
				}
			})
		}
	}
}

func TestFakeClockContextErrConcurrent(t *testing.T) {
	t.Parallel()
	ctx, cancel := WithTimeout(context.Background(), NewFakeClock(), time.Hour)
	defer cancel()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				if err := ctx.Err(); err != nil && err != context.Canceled {
					t.Errorf("Err() = %v, want nil or context.Canceled", err)
				}
			}
		}()
	}
	close(start)
	cancel()
	wg.Wait()
	<-ctx.Done()
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("Err() after cancellation = %v, want context.Canceled", err)
	}
}
