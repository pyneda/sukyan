package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWsFuzzBroadcasters_PerRunIsolation(t *testing.T) {
	r := newWsFuzzBroadcasters()
	b1 := r.Acquire(1)
	b2 := r.Acquire(2)
	require.NotNil(t, b1)
	require.NotNil(t, b2)
	require.NotSame(t, b1, b2)

	// Second Acquire(1) returns the same instance.
	require.Same(t, b1, r.Acquire(1))
}

func TestWsFuzzBroadcasters_LookupReturnsRegisteredOrNil(t *testing.T) {
	r := newWsFuzzBroadcasters()
	require.Nil(t, r.Lookup(7))
	b := r.Acquire(7)
	require.Same(t, b, r.Lookup(7))
}

func TestWsFuzzBroadcasters_ReleaseAllowsFreshAcquire(t *testing.T) {
	r := newWsFuzzBroadcasters()
	b1 := r.Acquire(1)
	r.Release(1)
	require.Nil(t, r.Lookup(1))
	b2 := r.Acquire(1)
	require.NotNil(t, b2)
	require.NotSame(t, b1, b2, "post-release Acquire should hand out a fresh broadcaster")
}
