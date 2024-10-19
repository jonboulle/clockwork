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

	ctx = AddToContext(ctx, NewRealClock())
	assertIsType(t, NewRealClock(), FromContext(ctx))
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
				t.Fatalf("WithDeadline context finished early.")
			default:
			}

			tc.action(cancelParent, cancelChild, clock)

			select {
			case <-child.Done():
				if got := child.Err(); !errors.Is(got, tc.want) {
					t.Errorf("WithDeadline context returned %v, want %v", got, tc.want)
				}
			case <-base.Done():
				t.Errorf("WithDeadline context was never canceled.")
			}
		})
	}
}

func TestWithDeadlineParentDeadlineDoesNotCancelChild(t *testing.T) {
	t.Parallel()
	base, cancelBase := context.WithTimeout(context.Background(), timeout)
	defer cancelBase()

	// Parent context hits deadline effectively immediately.
	parent, cancelParent := context.WithTimeout(base, time.Nanosecond)
	defer cancelParent()

	clock := NewFakeClockAt(time.Unix(10, 0))
	child, cancelChild := WithDeadline(parent, clock, time.Unix(20, 0))
	defer cancelChild()

	// TODO(https://github.com/jonboulle/clockwork/issues/67): The time.After()
	// below makes the case for having a way to validate that no timers have
	// fired, rather than waiting an arbitrary amount of time and hoping you've
	// waiting long enough to cover any race conditions.
	//
	// An abandoned attempt to do this can be found in
	// https://github.com/jonboulle/clockwork/pull/69.
	select {
	case <-time.After(50 * time.Millisecond): // Sleeping in tests, yuck.
	case <-child.Done():
		t.Errorf("WithDeadline context respected parenet deadline, returning %v, want parent deadline to be ignored.", child.Err())
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
			name:    "advancing past timeout cancels child",
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
				t.Fatalf("WithTimeout context finished early.")
			default:
			}

			tc.action(cancelParent, cancelChild, clock)

			select {
			case <-child.Done():
				if got := child.Err(); !errors.Is(got, tc.want) {
					t.Errorf("WithTimeout context returned %v, want %v", got, tc.want)
				}
			case <-background.Done():
				t.Errorf("WithTimeout context was never canceled.")
			}
		})
	}
}

func TestWithTimeoutParentTimeoutDoesNotCancelChild(t *testing.T) {
	t.Parallel()
	base, cancelBase := context.WithTimeout(context.Background(), timeout)
	defer cancelBase()

	// Parent context hits deadline effectively immediately.
	parent, cancelParent := context.WithTimeout(base, time.Nanosecond)
	defer cancelParent()

	clock := NewFakeClockAt(time.Unix(10, 0))
	child, cancelChild := WithTimeout(parent, clock, 10*time.Second)
	defer cancelChild()

	// TODO(https://github.com/jonboulle/clockwork/issues/67): The time.After()
	// below makes the case for having a way to validate that no timers have
	// fired, rather than waiting an arbitrary amount of time and hoping you've
	// waiting long enough to cover any race conditions.
	//
	// An abandoned attempt to do this can be found in
	// https://github.com/jonboulle/clockwork/pull/69.
	select {
	case <-time.After(50 * time.Millisecond): // Sleeping in tests, yuck.
	case <-child.Done():
		t.Errorf("WithTimeout context respected parenet deadline, returning %v, want parent deadline to be ignored.", child.Err())
	}
}
