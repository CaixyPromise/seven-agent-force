package key

import "time"

type Expire interface {
	Duration() time.Duration
	Permanent() bool
}

type TTL struct {
	duration  time.Duration
	permanent bool
}

func NewTTL(duration time.Duration) TTL {
	if duration <= 0 {
		return PermanentTTL()
	}
	return TTL{duration: duration}
}

func PermanentTTL() TTL {
	return TTL{permanent: true}
}

func (t TTL) Duration() time.Duration {
	if t.permanent {
		return 0
	}
	return t.duration
}

func (t TTL) Permanent() bool {
	return t.permanent || t.duration <= 0
}
