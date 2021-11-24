package clockwork

import (
	"context"
	"reflect"
	"testing"
)

func TestContextOps(t *testing.T) {
	ctx := context.Background()
	assertIsType(t, NewRealClock(), From(ctx))

	ctx = AddTo(ctx, NewFakeClock())
	assertIsType(t, NewFakeClock(), From(ctx))

	ctx = AddTo(ctx, NewRealClock())
	assertIsType(t, NewRealClock(), From(ctx))
}

func assertIsType(t *testing.T, expectedType interface{}, object interface{}) {
	t.Helper()

	if reflect.TypeOf(object) != reflect.TypeOf(expectedType) {
		t.Fatalf("Object expected to be of type %v, but was %v", reflect.TypeOf(expectedType), reflect.TypeOf(object))
	}
}
