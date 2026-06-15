package ratelimit

import (
	"sync"

	"golang.org/x/time/rate"
)

// Limiter holds one token-bucket limiter per key (API-key label).
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
}

func New() *Limiter {
	return &Limiter{buckets: make(map[string]*rate.Limiter)}
}

// Allow reports whether a request for key is permitted right now. The first
// time a key is seen, a bucket is created with the given rps/burst.
func (l *Limiter) Allow(key string, rps float64, burst int) bool {
	l.mu.Lock()
	lim, ok := l.buckets[key]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rps), burst)
		l.buckets[key] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}
