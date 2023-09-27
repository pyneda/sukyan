package http_utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	DEFAULT_MAXTOKENS = 100.0
	DEFAULT_RATE      = 10.0
)

func TestTokenBucketInitialization(t *testing.T) {
	tb := NewTokenBucket(DEFAULT_RATE, DEFAULT_MAXTOKENS, 0) // Using 0 to get default minRate
	assert.Equal(t, float64(DEFAULT_MAXTOKENS), tb.tokens)
	assert.Equal(t, float64(MIN_RATE), tb.minRate) // Should set to default minRate

	tbWithMinRate := NewTokenBucket(DEFAULT_RATE, DEFAULT_MAXTOKENS, 2.0)
	assert.Equal(t, float64(2.0), tbWithMinRate.minRate)
}

func TestTokenConsumption(t *testing.T) {
	tb := NewTokenBucket(DEFAULT_RATE, DEFAULT_MAXTOKENS, 0)
	assert.True(t, tb.HasToken())
	assert.Equal(t, DEFAULT_MAXTOKENS-1, tb.tokens)
	time.Sleep(time.Second) // Wait for tokens to refill
	assert.True(t, tb.HasToken())
}

func TestTokenRefill(t *testing.T) {
	tb := NewTokenBucket(DEFAULT_RATE, DEFAULT_MAXTOKENS, 0)

	// Consume the initial token
	assert.True(t, tb.HasToken())

	// Wait for a token to refill
	time.Sleep(100 * time.Millisecond)

	assert.True(t, tb.HasToken())
	time.Sleep(time.Second) // Wait for tokens to refill
	assert.True(t, tb.HasToken())
}

func TestRateAdjustment(t *testing.T) {
	tb := NewTokenBucket(DEFAULT_RATE, DEFAULT_MAXTOKENS, 0)

	// Adjust to a value above minRate
	tb.AdjustRate(5.0)
	assert.Equal(t, 5.0, tb.rate)

	// Adjust to a value below minRate
	tb.AdjustRate(0.5)
	assert.Equal(t, tb.minRate, tb.rate)
}
