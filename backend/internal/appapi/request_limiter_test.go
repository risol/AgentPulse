package appapi

import (
	"testing"
	"time"
)

func TestRequestRateLimiterRefillsBurstAtConfiguredRate(t *testing.T) {
	currentTime := time.Unix(100, 0)
	limiter := newRequestRateLimiter(2, 2, func() time.Time { return currentTime })

	if !limiter.Allow() || !limiter.Allow() || limiter.Allow() {
		t.Fatal("limiter did not enforce the initial burst")
	}
	currentTime = currentTime.Add(499 * time.Millisecond)
	if limiter.Allow() {
		t.Fatal("limiter refilled before one token was available")
	}
	currentTime = currentTime.Add(time.Millisecond)
	if !limiter.Allow() {
		t.Fatal("limiter did not refill one token after half a second")
	}
}
