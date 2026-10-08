package client

import (
	"testing"
	"time"
)

func TestBackoffJitterAndReset(t *testing.T) {
	b := newBackoff(100*time.Millisecond, 1*time.Second, 2.0)

	// First backoff: expected range [80ms, 120ms] (100ms ± 20%)
	d1 := b.duration()
	if d1 < 80*time.Millisecond || d1 > 120*time.Millisecond {
		t.Errorf("1st backoff duration out of expected range: %v", d1)
	}

	// Second backoff: base is 200ms, expected range [160ms, 240ms] (200ms ± 20%)
	d2 := b.duration()
	if d2 < 160*time.Millisecond || d2 > 240*time.Millisecond {
		t.Errorf("2nd backoff duration out of expected range: %v", d2)
	}

	// After reset: base restored to 100ms, expected range [80ms, 120ms]
	b.reset()
	dReset := b.duration()
	if dReset < 80*time.Millisecond || dReset > 120*time.Millisecond {
		t.Errorf("backoff duration after reset() out of expected range: %v", dReset)
	}
}
