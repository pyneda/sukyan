// Package ratelimit provides rate limiting interfaces for the scan engine.
package ratelimit

import (
	"context"
)

// RateLimiter defines the interface for rate limiting scan requests.
type RateLimiter interface {
	// Acquire blocks until a request can proceed, or returns error if cancelled.
	// scanID and host are used for per-scan and per-host limiting.
	Acquire(ctx context.Context, scanID uint, host string) error

	// Release signals completion of a request (for concurrency limiters).
	Release(scanID uint, host string)
}

// NoOpRateLimiter is a rate limiter that does nothing (allows all requests immediately).
type NoOpRateLimiter struct{}

// NewNoOpRateLimiter creates a new no-op rate limiter.
func NewNoOpRateLimiter() *NoOpRateLimiter {
	return &NoOpRateLimiter{}
}

// Acquire immediately returns nil (no limiting).
func (n *NoOpRateLimiter) Acquire(ctx context.Context, scanID uint, host string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Release does nothing.
func (n *NoOpRateLimiter) Release(scanID uint, host string) {}
