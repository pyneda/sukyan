package executor

import "sync"

// maxRetainedClaimScans bounds how many scans a ScanClaims keeps state for.
//
// Nothing in the scan lifecycle calls back into the executors when a scan
// finishes - the orchestrator owns completion and does not know about them - so
// the store evicts the oldest scan once the limit is reached rather than relying
// on a cleanup that may never run. The limit is far above the number of scans a
// worker has in flight, so an evicted scan is one that finished long ago.
const maxRetainedClaimScans = 64

// ScanClaims tracks once-per-scan claims on a string key, so an audit whose
// subject is broader than a single history item (a route, a host) runs once
// instead of once per item.
//
// A single instance is shared by every executor that can reach the same target,
// so a route claimed by an API scan job is not probed again by an active scan
// job in the same scan.
//
// Claims are process-local: with several worker processes on one scan each will
// claim independently. That bounds duplicate work to the number of processes
// rather than the number of history items, which is the defect worth fixing here.
type ScanClaims struct {
	mu     sync.Mutex
	claims map[uint]map[string]int
	order  []uint
}

func NewScanClaims() *ScanClaims {
	return &ScanClaims{claims: make(map[uint]map[string]int)}
}

// Claim reports whether key still needs handling for this scan, marking it
// atomically so exactly one concurrent caller wins.
func (c *ScanClaims) Claim(scanID uint, key string) bool {
	return c.ClaimUpTo(scanID, key, 1)
}

// ClaimUpTo reports whether key has been handled fewer than max times for this
// scan, counting this call when it returns true.
func (c *ScanClaims) ClaimUpTo(scanID uint, key string, max int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.claims[scanID] == nil {
		c.claims[scanID] = make(map[string]int)
		c.order = append(c.order, scanID)
		for len(c.order) > maxRetainedClaimScans {
			delete(c.claims, c.order[0])
			c.order = c.order[1:]
		}
	}
	if c.claims[scanID][key] >= max {
		return false
	}
	c.claims[scanID][key]++
	return true
}

// Release returns a single claimed key to the pool. It is used when a claim was
// taken to prevent a stampede but nothing was ultimately decided about the key,
// so a retry or a sibling job can still handle it.
func (c *ScanClaims) Release(scanID uint, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.claims[scanID] == nil {
		return
	}
	if c.claims[scanID][key] > 0 {
		c.claims[scanID][key]--
	}
}

// ReleaseScan drops every claim for a finished scan.
func (c *ScanClaims) ReleaseScan(scanID uint) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.claims, scanID)
	for i, id := range c.order {
		if id == scanID {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}
