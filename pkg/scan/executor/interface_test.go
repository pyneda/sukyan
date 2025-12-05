package executor

import (
	"context"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/stretchr/testify/assert"
)

// MockExecutor is a test executor
type MockExecutor struct {
	jobType      db.ScanJobType
	executeCount int
	lastJob      *db.ScanJob
}

func (m *MockExecutor) JobType() db.ScanJobType {
	return m.jobType
}

func (m *MockExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	m.executeCount++
	m.lastJob = job
	return nil
}

func TestExecutorRegistry_RegisterAndGet(t *testing.T) {
	registry := NewExecutorRegistry()

	// Create mock executor
	mockExec := &MockExecutor{jobType: db.ScanJobTypeActiveScan}

	// Register
	registry.Register(mockExec)

	// Get
	exec, ok := registry.Get(db.ScanJobTypeActiveScan)
	assert.True(t, ok, "Should find registered executor")
	assert.Equal(t, mockExec, exec, "Should return same executor")

	// Get non-existent
	_, ok = registry.Get(db.ScanJobTypeDiscovery)
	assert.False(t, ok, "Should not find unregistered executor")
}

func TestExecutorRegistry_MultipleExecutors(t *testing.T) {
	registry := NewExecutorRegistry()

	// Register multiple executors
	activeExec := &MockExecutor{jobType: db.ScanJobTypeActiveScan}
	wsExec := &MockExecutor{jobType: db.ScanJobTypeWebSocketScan}
	discoveryExec := &MockExecutor{jobType: db.ScanJobTypeDiscovery}

	registry.Register(activeExec)
	registry.Register(wsExec)
	registry.Register(discoveryExec)

	// Verify all can be retrieved
	exec, ok := registry.Get(db.ScanJobTypeActiveScan)
	assert.True(t, ok)
	assert.Equal(t, activeExec, exec)

	exec, ok = registry.Get(db.ScanJobTypeWebSocketScan)
	assert.True(t, ok)
	assert.Equal(t, wsExec, exec)

	exec, ok = registry.Get(db.ScanJobTypeDiscovery)
	assert.True(t, ok)
	assert.Equal(t, discoveryExec, exec)
}

func TestDefaultRegistry(t *testing.T) {
	// Test default registry functions
	mockExec := &MockExecutor{jobType: db.ScanJobTypeNuclei}

	// Clear any previous registrations
	DefaultRegistry = NewExecutorRegistry()

	RegisterExecutor(mockExec)

	exec, ok := GetExecutor(db.ScanJobTypeNuclei)
	assert.True(t, ok)
	assert.Equal(t, mockExec, exec)
}

func TestExecutorRegistry_OverwriteRegistration(t *testing.T) {
	registry := NewExecutorRegistry()

	// Register first executor
	exec1 := &MockExecutor{jobType: db.ScanJobTypeActiveScan}
	registry.Register(exec1)

	// Register second executor with same type
	exec2 := &MockExecutor{jobType: db.ScanJobTypeActiveScan}
	registry.Register(exec2)

	// Should get the second one
	exec, ok := registry.Get(db.ScanJobTypeActiveScan)
	assert.True(t, ok)
	assert.Equal(t, exec2, exec, "Should return the latest registered executor")
}
