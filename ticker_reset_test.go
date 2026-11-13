package clockwork

import (
	"testing"
	"time"
)

func TestFakeTickerResetThenStop(t *testing.T) {
	fc := NewFakeClock()
	ticker := fc.NewTicker(time.Second)
	ticker.Reset(2 * time.Second)
	ticker.Reset(3 * time.Second)
	ticker.Stop()
	fc.Advance(10 * time.Second)
	select {
	case tick := <-ticker.Chan():
		t.Fatalf("stopped ticker fired at %v after being reset", tick)
	default:
	}
	ticker.Reset(2 * time.Second)
	defer ticker.Stop()
	fc.Advance(2 * time.Second)
	select {
	case <-ticker.Chan():
	default:
		t.Fatal("Reset did not restart the stopped ticker")
	}
}

func TestFakeTickerResetNonPositive(t *testing.T) {
	for _, duration := range []time.Duration{0, -time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			fc := NewFakeClock()
			ticker := fc.NewTicker(time.Second)
			defer ticker.Stop()
			panicked := false
			func() {
				defer func() { panicked = recover() != nil }()
				ticker.Reset(duration)
			}()
			if !panicked {
				t.Fatal("Reset must panic for a non-positive duration")
			}
			fc.Advance(time.Second)
			select {
			case <-ticker.Chan():
			default:
				t.Fatal("invalid Reset changed the existing ticker schedule")
			}
		})
	}
}
