package mcpserver

import "sync"

type applicationLimiter struct {
	mu      sync.Mutex
	limit   int
	entries map[int64]*limiterEntry
}

type limiterEntry struct {
	active int
}

func newApplicationLimiter(limit int) *applicationLimiter {
	return &applicationLimiter{limit: limit, entries: make(map[int64]*limiterEntry)}
}

func (l *applicationLimiter) acquire(applicationID int64) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[applicationID]
	if entry == nil {
		entry = &limiterEntry{}
		l.entries[applicationID] = entry
	}
	if entry.active >= l.limit {
		if entry.active == 0 {
			delete(l.entries, applicationID)
		}
		return nil, false
	}
	entry.active++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		current := l.entries[applicationID]
		if current == nil {
			return
		}
		current.active--
		if current.active == 0 {
			delete(l.entries, applicationID)
		}
	}, true
}

func (l *applicationLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
