package domain

import "time"

// EffectivePriority implements the public scheduling contract. The injected
// instant lets tests exercise aging without sleeping; PostgreSQL uses the same
// expression with the Store clock passed as a query parameter.
func EffectivePriority(priority int, createdAt, now time.Time) int {
	if priority < 0 {
		priority = 0
	}
	if priority > 100 {
		return 100
	}
	if now.Before(createdAt) {
		return priority
	}
	priority += int(now.Sub(createdAt) / (5 * time.Minute))
	if priority > 100 {
		return 100
	}
	return priority
}
