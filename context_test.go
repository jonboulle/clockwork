package clockwork

import (
	"context"
	"reflect"
	"testing"
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
