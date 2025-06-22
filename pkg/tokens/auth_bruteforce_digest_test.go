package tokens

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/assert"
)

func TestDigestAuthBruteforce(t *testing.T) {
	// Create a test server that accepts specific credentials for digest auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && validateDigestAuth(authHeader, "admin", "password", "GET", r.URL.Path) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		} else {
			w.Header().Set("WWW-Authenticate", `Digest realm="Test Realm", nonce="12345", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}
	}))
	defer server.Close()

	// Create a proper raw request
	rawRequest := []byte("GET /protected HTTP/1.1\r\n" +
		"Host: " + server.URL[7:] + "\r\n" + // Remove http:// prefix
		"User-Agent: test-agent\r\n" +
		"\r\n")

	// Create a history item with proper raw request
	historyItem := &db.History{
		Method:      "GET",
		URL:         server.URL + "/protected",
		RawRequest:  rawRequest,
		WorkspaceID: func() *uint { id := uint(1); return &id }(),
		TaskID:      func() *uint { id := uint(1); return &id }(),
	}

	// Configure bruteforce with limited attempts for testing
	config := AuthBruteforceConfig{
		AuthType:      AuthTypeDigest,
		Mode:          "embedded",
		Format:        "pairs",
		Size:          "default",
		Concurrency:   2,
		MaxAttempts:   1000, // Increase to ensure we find admin/password
		StopOnSuccess: true,
	}

	// Run bruteforce
	result := BruteforceAuth(historyItem, `Digest realm="Test Realm", nonce="12345", qop="auth"`, config)

	// Assertions
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Found, "Should find valid credentials")
	assert.Equal(t, "admin", result.Username, "Should find correct username")
	assert.Equal(t, "password", result.Password, "Should find correct password")
	assert.Equal(t, http.StatusOK, result.StatusCode, "Should return success status")
	assert.Greater(t, result.Attempts, 0, "Should have made some attempts")
	assert.Greater(t, result.Duration, time.Duration(0), "Should have taken some time")
}

func TestCreateDigestAuthWithCounter(t *testing.T) {
	authHeader := `Digest realm="test", nonce="abc123", qop="auth"`

	// Test with counter = 1
	digestAuth1, err := createDigestAuthWithCounter("user", "pass", authHeader, "GET", "/test", 1)
	assert.NoError(t, err)
	assert.Contains(t, digestAuth1, `nc=00000001`)

	// Test with counter = 255 (0xFF)
	digestAuth255, err := createDigestAuthWithCounter("user", "pass", authHeader, "GET", "/test", 255)
	assert.NoError(t, err)
	assert.Contains(t, digestAuth255, `nc=000000ff`)

	// Test with counter = 4096 (0x1000)
	digestAuth4096, err := createDigestAuthWithCounter("user", "pass", authHeader, "GET", "/test", 4096)
	assert.NoError(t, err)
	assert.Contains(t, digestAuth4096, `nc=00001000`)
}

func TestIsStaleNonce(t *testing.T) {
	// Test stale nonce detection
	staleHeader := `Digest realm="test", nonce="abc123", stale=true, qop="auth"`
	assert.True(t, isStaleNonce(staleHeader))

	// Test case insensitive stale detection
	staleHeaderUpper := `Digest realm="test", nonce="abc123", stale=TRUE, qop="auth"`
	assert.True(t, isStaleNonce(staleHeaderUpper))

	// Test fresh nonce
	freshHeader := `Digest realm="test", nonce="abc123", qop="auth"`
	assert.False(t, isStaleNonce(freshHeader))

	// Test stale=false
	falseStaleHeader := `Digest realm="test", nonce="abc123", stale=false, qop="auth"`
	assert.False(t, isStaleNonce(falseStaleHeader))
}

func TestExtractNonce(t *testing.T) {
	authHeader := `Digest realm="test", nonce="abc123def456", qop="auth"`
	nonce := extractNonce(authHeader)
	assert.Equal(t, "abc123def456", nonce)

	// Test with no nonce
	noNonceHeader := `Digest realm="test", qop="auth"`
	emptyNonce := extractNonce(noNonceHeader)
	assert.Equal(t, "", emptyNonce)
}

func TestDigestStateNeedsRefresh(t *testing.T) {
	// Test time-based refresh
	ds := &DigestState{
		currentNonce: "test123",
		nonceCount:   10,
		lastRefresh:  time.Now().Add(-3 * time.Minute), // 3 minutes ago
		staleCount:   0,
	}
	assert.True(t, ds.needsRefresh())

	// Test attempt-based refresh
	ds2 := &DigestState{
		currentNonce: "test123",
		nonceCount:   150, // Over the 100 limit
		lastRefresh:  time.Now(),
		staleCount:   0,
	}
	assert.True(t, ds2.needsRefresh())

	// Test stale-based refresh
	ds3 := &DigestState{
		currentNonce: "test123",
		nonceCount:   10,
		lastRefresh:  time.Now(),
		staleCount:   5, // Over the 3 limit
	}
	assert.True(t, ds3.needsRefresh())

	// Test no refresh needed
	ds4 := &DigestState{
		currentNonce: "test123",
		nonceCount:   10,
		lastRefresh:  time.Now(),
		staleCount:   0,
	}
	assert.False(t, ds4.needsRefresh())
}

func TestDigestAuthWithProperNonceManagement(t *testing.T) {
	var requestCount int
	var lastNC string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		authHeader := r.Header.Get("Authorization")

		if authHeader != "" {
			// Extract nc from the auth header to verify it's incrementing
			if strings.Contains(authHeader, "nc=") {
				start := strings.Index(authHeader, "nc=") + 3
				end := start + 8 // nc is 8 characters
				if end <= len(authHeader) {
					lastNC = authHeader[start:end]
				}
			}

			// Accept credentials for testing
			if validateDigestAuthCredentials(authHeader, "admin", "password", "GET", r.URL.Path) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
				return
			}
		}

		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="test", nonce="nonce%d", qop="auth"`, requestCount))
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	// Create history item
	rawRequest := []byte("GET /test HTTP/1.1\r\nHost: " + server.URL[7:] + "\r\n\r\n")
	historyItem := &db.History{
		Method:      "GET",
		URL:         server.URL + "/test",
		RawRequest:  rawRequest,
		WorkspaceID: func() *uint { id := uint(1); return &id }(),
		TaskID:      func() *uint { id := uint(1); return &id }(),
	}

	// Test with digest state management
	digestState := &DigestState{
		currentNonce: "nonce1",
		nonceCount:   0,
		lastRefresh:  time.Now(),
	}

	historyOptions := http_utils.HistoryCreationOptions{
		WorkspaceID: 1,
		TaskID:      1,
	}

	// First attempt should use nc=00000001
	success, statusCode, _, err := attemptDigestAuth(historyItem, "admin", "password", digestState, historyOptions)
	assert.NoError(t, err)
	assert.True(t, success)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "00000001", lastNC)

	// Second attempt should increment to nc=00000002
	success2, statusCode2, _, err2 := attemptDigestAuth(historyItem, "admin", "password", digestState, historyOptions)
	assert.NoError(t, err2)
	assert.True(t, success2)
	assert.Equal(t, http.StatusOK, statusCode2)
	assert.Equal(t, "00000002", lastNC)
}

func TestRequestFreshChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not have Authorization header for challenge requests
		authHeader := r.Header.Get("Authorization")
		assert.Empty(t, authHeader, "Challenge request should not include Authorization header")

		w.Header().Set("WWW-Authenticate", `Digest realm="test", nonce="fresh123", qop="auth"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	rawRequest := []byte("GET /test HTTP/1.1\r\nHost: " + server.URL[7:] + "\r\n\r\n")
	historyItem := &db.History{
		Method:     "GET",
		URL:        server.URL + "/test",
		RawRequest: rawRequest,
	}

	historyOptions := http_utils.HistoryCreationOptions{
		WorkspaceID: 1,
		TaskID:      1,
	}

	challenge, err := requestFreshChallenge(historyItem, historyOptions)
	assert.NoError(t, err)
	assert.Contains(t, challenge, `realm="test"`)
	assert.Contains(t, challenge, `nonce="fresh123"`)
	assert.Contains(t, challenge, `qop="auth"`)
}

func TestIsQopAuth(t *testing.T) {
	// Test qop=auth
	authHeader := `Digest realm="test", nonce="abc123", qop="auth"`
	assert.True(t, isQopAuth(authHeader))

	// Test qop=auth-int
	authIntHeader := `Digest realm="test", nonce="abc123", qop="auth-int"`
	assert.True(t, isQopAuth(authIntHeader))

	// Test no qop
	noQopHeader := `Digest realm="test", nonce="abc123"`
	assert.False(t, isQopAuth(noQopHeader))

	// Test unsupported qop
	unsupportedQopHeader := `Digest realm="test", nonce="abc123", qop="auth-conf"`
	assert.False(t, isQopAuth(unsupportedQopHeader))
}

// Helper function to validate digest auth credentials in tests
func validateDigestAuthCredentials(authHeader, expectedUsername, expectedPassword, method, uri string) bool {
	if !strings.HasPrefix(authHeader, "Digest ") {
		return false
	}

	params := ParseDigestParams(authHeader)

	username, ok := params["username"]
	if !ok || username != expectedUsername {
		return false
	}

	realm, hasRealm := params["realm"]
	nonce, hasNonce := params["nonce"]
	response, hasResponse := params["response"]
	digestUri, hasUri := params["uri"]

	if !hasRealm || !hasNonce || !hasResponse || !hasUri {
		return false
	}

	// Calculate expected response
	ha1 := md5Hash(expectedUsername + ":" + realm + ":" + expectedPassword)
	ha2 := md5Hash(method + ":" + digestUri)

	var expectedResponse string
	if qop, hasQop := params["qop"]; hasQop && qop == "auth" {
		nc, hasNc := params["nc"]
		cnonce, hasCnonce := params["cnonce"]
		if hasNc && hasCnonce {
			expectedResponse = md5Hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
		} else {
			return false
		}
	} else {
		expectedResponse = md5Hash(ha1 + ":" + nonce + ":" + ha2)
	}

	return response == expectedResponse
}
