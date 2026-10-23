package appapi

import (
	"math"
	"sync"
	"time"
)

const (
	apiRequestsPerSecond = 120
	apiRequestBurst      = 240
)

type requestRateLimiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func newRequestRateLimiter(requestsPerSecond, burst int, now func() time.Time) *requestRateLimiter {
	if requestsPerSecond < 0 {
		requestsPerSecond = 0
	}
	if burst < 0 {
		burst = 0
	}
	if now == nil {
		now = time.Now
	}
	currentTime := now()
	return &requestRateLimiter{
		rate: float64(requestsPerSecond), burst: float64(burst),
		tokens: float64(burst), last: currentTime, now: now,
	}
}

func (l *requestRateLimiter) Allow() bool {
	currentTime := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if elapsed := currentTime.Sub(l.last).Seconds(); elapsed > 0 {
		l.tokens = math.Min(l.burst, l.tokens+elapsed*l.rate)
		l.last = currentTime
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
