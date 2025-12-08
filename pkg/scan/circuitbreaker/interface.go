package circuitbreaker

// Action represents the action to take after recording a failure.
type Action int

const (
	// ActionContinue means keep going normally.
	ActionContinue Action = iota
	// ActionThrottle means slow down requests.
	ActionThrottle
	// ActionPauseScan means auto-pause the scan due to too many failures.
	ActionPauseScan
	// ActionBlockHost means skip this host temporarily.
	ActionBlockHost
)

// String returns the string representation of the action.
func (a Action) String() string {
	switch a {
	case ActionContinue:
		return "continue"
	case ActionThrottle:
		return "throttle"
	case ActionPauseScan:
		return "pause_scan"
	case ActionBlockHost:
		return "block_host"
	default:
		return "unknown"
	}
}

// CircuitBreaker defines the interface for circuit breaking on failures.
type CircuitBreaker interface {
	// RecordSuccess resets failure counter for successful requests.
	RecordSuccess(scanID uint, host string)

	// RecordFailure increments counter and returns action to take.
	// errType classifies the error (e.g., "timeout", "connection_refused", "rate_limited").
	RecordFailure(scanID uint, host string, errType string) Action
}

// NoOpCircuitBreaker is a circuit breaker that does nothing.
type NoOpCircuitBreaker struct{}

// NewNoOpCircuitBreaker creates a new no-op circuit breaker.
func NewNoOpCircuitBreaker() *NoOpCircuitBreaker {
	return &NoOpCircuitBreaker{}
}

// RecordSuccess does nothing.
func (n *NoOpCircuitBreaker) RecordSuccess(scanID uint, host string) {}

// RecordFailure always returns ActionContinue.
func (n *NoOpCircuitBreaker) RecordFailure(scanID uint, host string, errType string) Action {
	return ActionContinue
}
