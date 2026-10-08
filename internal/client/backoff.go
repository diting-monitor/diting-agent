package client

import (
	"math/rand"
	"time"
)

// backoff implements an exponential backoff algorithm with randomized jitter (±20%)
// to prevent thundering herd problems during mass agent reconnections.
type backoff struct {
	min    time.Duration
	max    time.Duration
	factor float64
	curr   time.Duration
}

// newBackoff creates a new backoff instance.
// minDuration: initial backoff interval (e.g. 1s).
// maxDuration: maximum backoff ceiling (e.g. 30s).
// factor: multiplication factor applied on each failure (e.g. 2.0).
func newBackoff(minDuration, maxDuration time.Duration, factor float64) *backoff {
	if minDuration <= 0 {
		minDuration = 1 * time.Second
	}
	if maxDuration < minDuration {
		maxDuration = minDuration
	}
	if factor <= 1.0 {
		factor = 2.0
	}
	return &backoff{
		min:    minDuration,
		max:    maxDuration,
		factor: factor,
		curr:   minDuration,
	}
}

// duration calculates the next backoff duration applying a ±20% jitter.
func (b *backoff) duration() time.Duration {
	d := b.curr

	// Exponential increment with upper ceiling capping (using Go 1.21+ built-in min).
	next := time.Duration(float64(b.curr) * b.factor)
	b.curr = min(next, b.max)

	// Apply ±20% jitter [0.8, 1.2) to distribute reconnection requests across time.
	jitter := float64(d) * (0.8 + 0.4*rand.Float64())
	if jitter <= 0 {
		return b.min
	}
	return time.Duration(jitter)
}

// reset restores the backoff duration to the initial minimum value after a successful connection.
func (b *backoff) reset() {
	b.curr = b.min
}
