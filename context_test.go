package clockwork

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestContextOps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	assertIsType(t, NewRealClock(), FromContext(ctx))

	ctx = AddToContext(ctx, NewFakeClock())
	assertIsType(t, NewFakeClock(), FromContext(ctx))
}

func assertIsType(t *testing.T, expectedType, object any) {
	t.Helper()
	if reflect.TypeOf(object) != reflect.TypeOf(expectedType) {
		t.Fatalf("Object expected to be of type %v, but was %v", reflect.TypeOf(expectedType), reflect.TypeOf(object))
	}
}

func TestWithDeadlineDone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string

		start, deadline time.Time

		action func(cancelParent, cancelChild context.CancelFunc, clock *FakeClock)

		want error
	}{
		{
			name:     "canceling parent cancels child",
			start:    time.Unix(10, 0),
			deadline: time.Unix(20, 0),
			action: func(cancelParent, _ context.CancelFunc, _ *FakeClock) {
				cancelParent()
			},
			want: context.Canceled,
		},
		{
			name:     "canceling child cancels child",
			start:    time.Unix(10, 0),
			deadline: time.Unix(20, 0),
			action: func(_, cancelChild context.CancelFunc, _ *FakeClock) {
				cancelChild()
			},
			want: context.Canceled,
		},
		{
			name:     "advancing past deadline cancels child",
			start:    time.Unix(10, 0),
			deadline: time.Unix(20, 0),
			action: func(_, _ context.CancelFunc, clock *FakeClock) {
				clock.Advance(10 * time.Second)
			},
			want: context.DeadlineExceeded,
		},
	}

	// Background context for catching timeouts.
	base, cancelBase := context.WithTimeout(context.Background(), timeout)
	defer cancelBase()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			parent, cancelParent := context.WithCancel(base)
			clock := NewFakeClockAt(tc.start)
			child, cancelChild := WithDeadline(parent, clock, tc.deadline)

			select {
			case <-child.Done():
				t.Fatal("WithDeadline context finished early.")
			default:
			}

			tc.action(cancelParent, cancelChild, clock)

			select {
			case <-child.Done():
				if got := child.Err(); !errors.Is(got, tc.want) {
					t.Errorf("WithDeadline context returned %v, want %v", got, tc.want)
				}
			case <-base.Done():
				t.Error("WithDeadline context was never canceled.")
			}
		})
	}
}
func TestWithTimeoutDone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string

		start   time.Time
		timeout time.Duration

		action func(cancelParent, cancelChild context.CancelFunc, clock *FakeClock)

		want error
	}{
		{
			name:    "canceling parent cancels child",
			start:   time.Unix(10, 0),
			timeout: 10 * time.Second,
			action: func(cancelParent, _ context.CancelFunc, _ *FakeClock) {
				cancelParent()
			},
			want: context.Canceled,
		},
		{
			name:    "canceling child cancels child",
			start:   time.Unix(10, 0),
			timeout: 10 * time.Second,
			action: func(_, cancelChild context.CancelFunc, _ *FakeClock) {
				cancelChild()
			},
			want: context.Canceled,
		},
		{
			name:    "advancing past deadline cancels child",
			start:   time.Unix(10, 0),
			timeout: 10 * time.Second,
			action: func(_, _ context.CancelFunc, clock *FakeClock) {
				clock.Advance(10 * time.Second)
			},
			want: context.DeadlineExceeded,
		},
	}

	// Background context for catching timeouts.
	background, cancelBase := context.WithTimeout(context.Background(), timeout)
	defer cancelBase()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			parent, cancelParent := context.WithCancel(background)
			clock := NewFakeClockAt(tc.start)
			child, cancelChild := WithTimeout(parent, clock, tc.timeout)

			select {
			case <-child.Done():
				t.Fatal("WithTimeout context finished early.")
			default:
			}

			tc.action(cancelParent, cancelChild, clock)

			select {
			case <-child.Done():
				if got := child.Err(); !errors.Is(got, tc.want) {
					t.Errorf("WithTimeout context returned %v, want %v", got, tc.want)
				}
			case <-background.Done():
				t.Error("WithTimeout context was never canceled.")
			}
		})
	}
}

func TestParentCancellationIsRespected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string

		contextFunc func(context.Context, *FakeClock) (context.Context, context.CancelFunc)

		requireContextDeadlineExceeded bool
	}{
		{
			name: "WithDeadline in the future",
			contextFunc: func(ctx context.Context, fc *FakeClock) (context.Context, context.CancelFunc) {
				return WithDeadline(ctx, fc, time.Now().Add(time.Hour))
			},
			// The FakeClock does not hit its deadline, so the error must be context.DeadlineExceeded.
			requireContextDeadlineExceeded: true,
		},
		{
			name: "WithDeadline in the past",
			contextFunc: func(ctx context.Context, fc *FakeClock) (context.Context, context.CancelFunc) {
				return WithDeadline(ctx, fc, time.Now().Add(-time.Hour))
			},
		},
		{
			name: "WithTimeout in the future",
			contextFunc: func(ctx context.Context, fc *FakeClock) (context.Context, context.CancelFunc) {
				return WithTimeout(ctx, fc, time.Hour)
			},
			// The FakeClock does not hit its deadline, so the error must be context.DeadlineExceeded.
			requireContextDeadlineExceeded: true,
		},
		{
			name: "WithTimeout immediately",
			contextFunc: func(ctx context.Context, fc *FakeClock) (context.Context, context.CancelFunc) {
				return WithTimeout(ctx, fc, 0)
			},
		},
		{
			name: "WithTimeout in the past",
			contextFunc: func(ctx context.Context, fc *FakeClock) (context.Context, context.CancelFunc) {
				return WithTimeout(ctx, fc, -time.Hour)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, cancelBase := context.WithTimeout(context.Background(), timeout)
			defer cancelBase()

			// Parent context hits deadline effectively immediately.
			parent, cancelParent := context.WithTimeout(base, time.Nanosecond)
			defer cancelParent()

			clock := NewFakeClockAt(time.Unix(10, 0))
			child, cancelChild := tc.contextFunc(parent, clock)
			defer cancelChild()

			select {
			case <-child.Done():
			case <-base.Done():
				t.Fatal("context did not respect parnet deadline")
			}

			if err := child.Err(); !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("errors.Is(Context.Err(), context.DeadlineExceeded) == falst, want true, error: %v", err)
			}
			if tc.requireContextDeadlineExceeded {
				if err := child.Err(); errors.Is(err, ErrFakeClockDeadlineExceeded) {
					t.Errorf("errors.Is(Context.Err(), ErrFakeClockDeadlineExceeded) == true, want false, error: %v", err)
				}
			}
		})
	}
}
