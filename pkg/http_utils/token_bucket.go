package http_utils

import (
	"sync"
	"time"
)

type TokenBucket struct {
	tokens      float64
	maxTokens   float64
	rate        float64
	lastUpdated time.Time
	mu          sync.Mutex
	minRate     float64
	// initialRate  float64
}

func NewTokenBucket(rate float64, maxTokens float64, minRate float64) *TokenBucket {
	if minRate == 0 {
		minRate = MIN_RATE
	}
	return &TokenBucket{
		tokens:      maxTokens,
		maxTokens:   maxTokens,
		rate:        rate,
		lastUpdated: time.Now(),
		minRate:     minRate,
		// initialRate: rate,
	}
}

func (tb *TokenBucket) AdjustRate(newRate float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	// Ensure rate is within bounds
	if newRate < tb.minRate {
		newRate = tb.minRate
	}
	tb.rate = newRate
}

func (tb *TokenBucket) HasToken() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Calculate tokens to add based on time passed
	now := time.Now()
	delta := now.Sub(tb.lastUpdated).Seconds()
	tb.tokens += delta * tb.rate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastUpdated = now

	// Check for available tokens
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}
