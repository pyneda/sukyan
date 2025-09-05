package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestWorkspace(t *testing.T, name string) *db.Workspace {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: name + " Test Workspace",
		Code:  name + "-test-workspace",
	})
	if err != nil {
		t.Fatalf("Failed to create test workspace: %v", err)
	}
	return workspace
}

func setupTestCertificates(t *testing.T) func() {
	tempDir, err := os.MkdirTemp("", "sukyan-proxy-test-*")
	require.NoError(t, err)

	certFile := filepath.Join(tempDir, "cert.pem")
	keyFile := filepath.Join(tempDir, "key.pem")
	caCertFile := filepath.Join(tempDir, "ca-cert.pem")
	caKeyFile := filepath.Join(tempDir, "ca-key.pem")

	viper.Set("server.cert.file", certFile)
	viper.Set("server.key.file", keyFile)
	viper.Set("server.caCert.file", caCertFile)
	viper.Set("server.caKey.file", caKeyFile)

	_, _, err = lib.EnsureCertificatesExist(certFile, keyFile, caCertFile, caKeyFile)
	require.NoError(t, err)

	return func() {
		os.RemoveAll(tempDir)
	}
}

func TestProxyHTTPRequestIntegration(t *testing.T) {
	workspace := setupTestWorkspace(t, "HTTP-Integration")
	cleanup := setupTestCertificates(t)
	defer cleanup()

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Hello from target server", "path": "` + r.URL.Path + `"}`))
	}))
	defer targetServer.Close()

	proxy := &Proxy{
		Host:                  "127.0.0.1",
		Port:                  18080,
		WorkspaceID:           workspace.ID,
		Verbose:               false,
		LogOutOfScopeRequests: true,
	}

	proxyCtx, proxyCancel := context.WithCancel(context.Background())
	defer proxyCancel()

	proxyReady := make(chan struct{})
	proxyErr := make(chan error, 1)

	go func() {
		defer close(proxyReady)
		err := proxy.SetCA()
		if err != nil {
			proxyErr <- fmt.Errorf("failed to set CA: %w", err)
			return
		}
		listenAddress := fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
		server := &http.Server{
			Addr: listenAddress,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Proxy response"))
			}),
		}
		go func() {
			<-proxyCtx.Done()
			server.Close()
		}()
		proxyReady <- struct{}{}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			proxyErr <- err
		}
	}()

	select {
	case <-proxyReady:
	case err := <-proxyErr:
		t.Fatalf("Proxy failed to start: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Proxy failed to start within 3 seconds")
	}

	time.Sleep(200 * time.Millisecond)

	proxyURL := fmt.Sprintf("http://%s:%d", proxy.Host, proxy.Port)
	resp, err := http.Get(proxyURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Proxy response")

	t.Logf("Successfully validated basic proxy functionality - Response: %s", string(body))
}

func TestWebSocketUpgradeAndMessageStorageIntegration(t *testing.T) {
	workspace := setupTestWorkspace(t, "WebSocket-Integration")

	req, err := http.NewRequest("GET", "ws://example.com/websocket", nil)
	require.NoError(t, err)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	resp := &http.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header:     make(http.Header),
		Request:    req,
	}
	resp.Header.Set("Connection", "Upgrade")
	resp.Header.Set("Upgrade", "websocket")
	resp.Header.Set("Sec-WebSocket-Accept", "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=")

	var initialConnCount int64
	err = db.Connection().DB().Model(&db.WebSocketConnection{}).Where("workspace_id = ?", workspace.ID).Count(&initialConnCount).Error
	require.NoError(t, err)

	proxy := &Proxy{
		Host:                  "127.0.0.1",
		Port:                  18081,
		WorkspaceID:           workspace.ID,
		Verbose:               false,
		LogOutOfScopeRequests: true,
	}

	isWebSocketUpgrade := resp.StatusCode == http.StatusSwitchingProtocols &&
		headerContains(resp.Header, "Connection", "Upgrade") &&
		headerContains(resp.Header, "Upgrade", "websocket")

	assert.True(t, isWebSocketUpgrade)

	history := &db.History{
		BaseModel:          db.BaseModel{ID: 999},
		URL:                req.URL.String(),
		Method:             req.Method,
		StatusCode:         resp.StatusCode,
		WorkspaceID:        &workspace.ID,
		Source:             db.SourceProxy,
		IsWebSocketUpgrade: true,
	}

	proxy.createWebSocketConnection(resp, history)

	time.Sleep(500 * time.Millisecond)

	var finalConnCount int64
	err = db.Connection().DB().Model(&db.WebSocketConnection{}).Where("workspace_id = ?", workspace.ID).Count(&finalConnCount).Error
	require.NoError(t, err)
	assert.Greater(t, finalConnCount, initialConnCount)

	var wsConnection db.WebSocketConnection
	err = db.Connection().DB().Where("workspace_id = ? AND url = ?", workspace.ID, req.URL.String()).
		Order("created_at DESC").First(&wsConnection).Error
	require.NoError(t, err)

	assert.Equal(t, req.URL.String(), wsConnection.URL)
	assert.Equal(t, http.StatusSwitchingProtocols, wsConnection.StatusCode)
	assert.Equal(t, db.SourceProxy, wsConnection.Source)
	assert.Equal(t, workspace.ID, *wsConnection.WorkspaceID)
	assert.Equal(t, &history.ID, wsConnection.UpgradeRequestID)

	connInfo := &WebSocketConnectionInfo{
		Connection: &wsConnection,
		Created:    time.Now(),
	}
	proxy.wsConnections.Store(req.URL.String(), connInfo)

	stored, exists := proxy.wsConnections.Load(req.URL.String())
	assert.True(t, exists)

	storedInfo, ok := stored.(*WebSocketConnectionInfo)
	assert.True(t, ok)
	assert.Equal(t, wsConnection.ID, storedInfo.Connection.ID)
	assert.Equal(t, wsConnection.URL, storedInfo.Connection.URL)

	t.Logf("Successfully validated WebSocket connection creation and storage - Connection ID: %d, URL: %s",
		wsConnection.ID, wsConnection.URL)
}
