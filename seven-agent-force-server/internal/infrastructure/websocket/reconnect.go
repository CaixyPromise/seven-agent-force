package websocket

import "time"

type backoff struct {
	initial time.Duration
	max     time.Duration
	current time.Duration
}

func newBackoff(initial, max time.Duration) *backoff {
	if initial <= 0 {
		initial = defaultReconnectInitialDelay
	}
	if max < initial {
		max = initial
	}
	return &backoff{
		initial: initial,
		max:     max,
	}
}

func (b *backoff) Next() time.Duration {
	if b.current <= 0 {
		b.current = b.initial
		return b.current
	}
	next := b.current * 2
	if next > b.max {
		next = b.max
	}
	b.current = next
	return b.current
}

func (b *backoff) Reset() {
	b.current = 0
}
