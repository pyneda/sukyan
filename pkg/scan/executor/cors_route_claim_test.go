package executor

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaimCORSRoute_ProbesEachRouteOncePerScan(t *testing.T) {
	e := NewActiveScanExecutor(nil, nil, nil)

	require.True(t, e.claimCORSRoute(1, "https://example.com/api/auth"))
	require.False(t, e.claimCORSRoute(1, "https://example.com/api/auth"), "second claim for the same route must be refused")
	require.True(t, e.claimCORSRoute(1, "https://example.com/api/wallet"), "a different route is still claimable")
}

func TestClaimCORSRoute_IsScopedPerScan(t *testing.T) {
	e := NewActiveScanExecutor(nil, nil, nil)

	require.True(t, e.claimCORSRoute(1, "https://example.com/api/auth"))
	require.True(t, e.claimCORSRoute(2, "https://example.com/api/auth"), "a different scan must probe independently")
}

func TestClaimCORSRoute_IsConcurrencySafe(t *testing.T) {
	e := NewActiveScanExecutor(nil, nil, nil)

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.claimCORSRoute(7, "https://example.com/api/auth") {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 1, granted, "exactly one concurrent claimant may win")
}

func TestCleanupScan_ReleasesCORSRoutes(t *testing.T) {
	e := NewActiveScanExecutor(nil, nil, nil)

	require.True(t, e.claimCORSRoute(3, "https://example.com/api/auth"))
	e.CleanupScan(3)
	require.True(t, e.claimCORSRoute(3, "https://example.com/api/auth"), "cleanup must not leak routes across scans")
}

func TestScanClaims_ClaimUpToAllowsExactlyMax(t *testing.T) {
	c := NewScanClaims()

	require.True(t, c.ClaimUpTo(1, "report:host|High|arbitrary-origin", 3))
	require.True(t, c.ClaimUpTo(1, "report:host|High|arbitrary-origin", 3))
	require.True(t, c.ClaimUpTo(1, "report:host|High|arbitrary-origin", 3))
	require.False(t, c.ClaimUpTo(1, "report:host|High|arbitrary-origin", 3), "the fourth must be refused")
}

func TestScanClaims_ReleaseReturnsASingleKey(t *testing.T) {
	c := NewScanClaims()

	require.True(t, c.Claim(1, "https://example.com/api/auth"))
	require.False(t, c.Claim(1, "https://example.com/api/auth"))

	c.Release(1, "https://example.com/api/auth")
	require.True(t, c.Claim(1, "https://example.com/api/auth"), "a released route must be claimable again")
}

func TestScanClaims_ReleaseOfAnUnclaimedKeyIsHarmless(t *testing.T) {
	c := NewScanClaims()

	c.Release(99, "never-claimed")
	require.True(t, c.Claim(99, "never-claimed"))
}

func TestScanClaims_EvictsOldestScansSoMemoryStaysBounded(t *testing.T) {
	c := NewScanClaims()

	for scanID := uint(1); scanID <= maxRetainedClaimScans+10; scanID++ {
		c.Claim(scanID, "route")
	}

	c.mu.Lock()
	retained := len(c.claims)
	c.mu.Unlock()

	require.LessOrEqual(t, retained, maxRetainedClaimScans,
		"nothing calls back on scan completion, so the store must bound itself")
	require.True(t, c.Claim(1, "route"), "the oldest scan's claims should have been evicted")
}
