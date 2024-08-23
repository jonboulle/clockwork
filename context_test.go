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

func assertIsType(t *testing.T, expectedType, object interface{}) {
	t.Helper()
	if reflect.TypeOf(object) != reflect.TypeOf(expectedType) {
		t.Fatalf("Object expected to be of type %v, but was %v", reflect.TypeOf(expectedType), reflect.TypeOf(object))
	}
}

// Ensure that WithDeadline is cancelled when deadline exceeded.
func TestFakeClock_WithDeadline(t *testing.T) {
	m := NewFakeClock()
	now := m.Now()
	ctx, _ := m.WithDeadline(context.Background(), now.Add(time.Second))
	m.Advance(time.Second)
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Error("invalid type of error returned when deadline exceeded")
		}
	default:
		t.Error("context is not cancelled when deadline exceeded")
	}
}

// Ensure that WithDeadline does nothing when the deadline is later than the current deadline.
func TestFakeClock_WithDeadlineLaterThanCurrent(t *testing.T) {
	m := NewFakeClock()
	ctx, _ := m.WithDeadline(context.Background(), m.Now().Add(time.Second))
	ctx, _ = m.WithDeadline(ctx, m.Now().Add(10*time.Second))
	m.Advance(time.Second)
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Error("invalid type of error returned when deadline exceeded")
		}
	default:
		t.Error("context is not cancelled when deadline exceeded")
	}
}

// Ensure that WithDeadline cancel closes Done channel with context.Canceled error.
func TestFakeClock_WithDeadlineCancel(t *testing.T) {
	m := NewFakeClock()
	ctx, cancel := m.WithDeadline(context.Background(), m.Now().Add(time.Second))
	cancel()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Error("invalid type of error returned after cancellation")
		}
	case <-time.After(time.Second):
		t.Error("context is not cancelled after cancel was called")
	}
}

// Ensure that WithDeadline closes child contexts after it was closed.
func TestFakeClock_WithDeadlineCancelledWithParent(t *testing.T) {
	m := NewFakeClock()
	parent, cancel := context.WithCancel(context.Background())
	ctx, _ := m.WithDeadline(parent, m.Now().Add(time.Second))
	cancel()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Error("invalid type of error returned after cancellation")
		}
	case <-time.After(time.Second):
		t.Error("context is not cancelled when parent context is cancelled")
	}
}

// Ensure that WithDeadline cancelled immediately when deadline has already passed.
func TestFakeClock_WithDeadlineImmediate(t *testing.T) {
	m := NewFakeClock()
	ctx, _ := m.WithDeadline(context.Background(), m.Now().Add(-time.Second))
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Error("invalid type of error returned when deadline has already passed")
		}
	default:
		t.Error("context is not cancelled when deadline has already passed")
	}
}

// Ensure that WithTimeout is cancelled when deadline exceeded.
func TestFakeClock_WithTimeout(t *testing.T) {
	m := NewFakeClock()
	ctx, _ := m.WithTimeout(context.Background(), time.Second)
	m.Advance(time.Second)
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Error("invalid type of error returned when time is over")
		}
	default:
		t.Error("context is not cancelled when time is over")
	}
}
