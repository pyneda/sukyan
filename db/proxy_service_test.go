package db

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freeTestProxyPort reserves a port that is unused in the global unique index
// on proxy_services.port and hard-deletes whatever row lands on it when the
// test ends. The index covers soft-deleted rows too, so a scoped delete would
// keep the port reserved and poison every later run.
func freeTestProxyPort(t *testing.T) int {
	t.Helper()
	for range 50 {
		port := 20000 + rand.IntN(40000)
		var taken int64
		require.NoError(t, Connection().DB().Unscoped().Model(&ProxyService{}).Where("port = ?", port).Count(&taken).Error)
		if taken > 0 {
			continue
		}
		t.Cleanup(func() {
			Connection().DB().Unscoped().Where("port = ?", port).Delete(&ProxyService{})
		})
		return port
	}
	t.Fatal("no free proxy service port found")
	return 0
}

func TestCreateProxyService(t *testing.T) {
	workspace := createTestWorkspace(t)
	port := freeTestProxyPort(t)

	proxyService := &ProxyService{
		WorkspaceID:           &workspace.ID,
		Name:                  "Test Proxy",
		Host:                  "localhost",
		Port:                  port,
		Verbose:               true,
		LogOutOfScopeRequests: true,
		Enabled:               false,
	}

	created, err := Connection().CreateProxyService(proxyService)
	require.NoError(t, err)
	assert.NotNil(t, created.ID)
	assert.Equal(t, "Test Proxy", created.Name)
	assert.Equal(t, port, created.Port)
}

func TestProxyServicePortUniqueness(t *testing.T) {
	workspace := createTestWorkspace(t)
	port := freeTestProxyPort(t)

	proxy1 := &ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 1",
		Port:        port,
	}
	_, err := Connection().CreateProxyService(proxy1)
	require.NoError(t, err)

	// Try to create another proxy with same port
	proxy2 := &ProxyService{
		WorkspaceID: &workspace.ID,
		Name:        "Proxy 2",
		Port:        port,
	}
	_, err = Connection().CreateProxyService(proxy2)
	assert.Error(t, err) // Should fail due to unique constraint
}
