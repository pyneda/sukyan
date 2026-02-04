package circuitbreaker

import "sync"

type AuthCircuitBreaker struct {
	mu               sync.Mutex
	consecutiveAuths map[uint]int
	threshold        int
}

func NewAuthCircuitBreaker(threshold int) *AuthCircuitBreaker {
	return &AuthCircuitBreaker{
		consecutiveAuths: make(map[uint]int),
		threshold:        threshold,
	}
}

func (a *AuthCircuitBreaker) RecordSuccess(scanID uint, host string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.consecutiveAuths[scanID] = 0
}

func (a *AuthCircuitBreaker) RecordFailure(scanID uint, host string, errType string) Action {
	if errType != "auth_failure" {
		return ActionContinue
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.consecutiveAuths[scanID]++
	if a.consecutiveAuths[scanID] >= a.threshold {
		a.consecutiveAuths[scanID] = 0
		return ActionPauseScan
	}

	return ActionContinue
}

func (a *AuthCircuitBreaker) Reset(scanID uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.consecutiveAuths, scanID)
}
