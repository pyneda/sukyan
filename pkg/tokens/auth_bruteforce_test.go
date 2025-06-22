package tokens

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestBasicAuthBruteforce(t *testing.T) {
	// Create a test server that accepts specific credentials
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok && username == "admin" && password == "password" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		} else {
			w.Header().Set("WWW-Authenticate", `Basic realm="Test Realm"`)
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
		AuthType:      AuthTypeBasic,
		Mode:          "embedded",
		Format:        "pairs",
		Size:          "default",
		Concurrency:   2,
		MaxAttempts:   1000, // Increase to ensure we find admin/password
		StopOnSuccess: true,
	}

	// Run bruteforce
	result := BruteforceAuth(historyItem, `Basic realm="Test Realm"`, config)

	// Assertions
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Found, "Should find valid credentials")
	assert.Equal(t, "admin", result.Username, "Should find correct username")
	assert.Equal(t, "password", result.Password, "Should find correct password")
	assert.Equal(t, http.StatusOK, result.StatusCode, "Should return success status")
	assert.Greater(t, result.Attempts, 0, "Should have made some attempts")
	assert.Greater(t, result.Duration, time.Duration(0), "Should have taken some time")
}

func TestAuthBruteforceNoCredentialsFound(t *testing.T) {
	// Create a test server that never accepts any credentials
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Test Realm"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
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

	// Configure bruteforce with very limited attempts
	config := AuthBruteforceConfig{
		AuthType:      AuthTypeBasic,
		Mode:          "embedded",
		Format:        "pairs",
		Size:          "default",
		Concurrency:   2,
		MaxAttempts:   5,
		StopOnSuccess: true,
	}

	// Run bruteforce
	result := BruteforceAuth(historyItem, `Basic realm="Test Realm"`, config)

	// Assertions
	assert.NotNil(t, result, "Result should not be nil")
	assert.False(t, result.Found, "Should not find valid credentials")
	assert.Equal(t, "", result.Username, "Username should be empty")
	assert.Equal(t, "", result.Password, "Password should be empty")
	assert.Equal(t, 5, result.Attempts, "Should have made exactly 5 attempts")
	assert.Greater(t, result.Duration, time.Duration(0), "Should have taken some time")
}

func TestDigestAuthParamExtraction(t *testing.T) {
	authHeader := `Digest realm="test realm", nonce="abc123", qop="auth", algorithm="MD5"`

	params := ParseDigestParams(authHeader)

	assert.Equal(t, "test realm", params["realm"], "Should extract realm correctly")
	assert.Equal(t, "abc123", params["nonce"], "Should extract nonce correctly")
	assert.Equal(t, "auth", params["qop"], "Should extract qop correctly")
	assert.Equal(t, "MD5", params["algorithm"], "Should extract algorithm correctly")
}

func TestDigestAuthCreation(t *testing.T) {
	username := "testuser"
	password := "testpass"
	authHeader := `Digest realm="test", nonce="12345", qop="auth"`
	method := "GET"
	uri := "/test"

	digestAuth, err := createDigestAuth(username, password, authHeader, method, uri)

	assert.NoError(t, err, "Should create digest auth without error")
	assert.Contains(t, digestAuth, "Digest", "Should start with Digest")
	assert.Contains(t, digestAuth, `username="testuser"`, "Should contain username")
	assert.Contains(t, digestAuth, `realm="test"`, "Should contain realm")
	assert.Contains(t, digestAuth, `nonce="12345"`, "Should contain nonce")
	assert.Contains(t, digestAuth, `uri="/test"`, "Should contain URI")
	assert.Contains(t, digestAuth, "response=", "Should contain response hash")
}

func TestLoadCredentialsFromEmbedded(t *testing.T) {
	credentials, err := loadCredentialsFromBytes(embeddedUsernamesDefault)

	assert.NoError(t, err, "Should load embedded credentials without error")
	assert.Greater(t, len(credentials), 0, "Should load some credentials")
	assert.Contains(t, credentials, "admin", "Should contain common username 'admin'")
	assert.Contains(t, credentials, "root", "Should contain common username 'root'")
}

// Test loading different wordlist sizes
func TestLoadUsernameWordlistSizes(t *testing.T) {
	config := AuthBruteforceConfig{
		Mode:   "embedded",
		Format: "separate",
	}

	// Test default size
	config.Size = "default"
	usernames, err := loadUsernameWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, usernames)
	defaultCount := len(usernames)

	// Test xs size (should be smaller)
	config.Size = "xs"
	usernames, err = loadUsernameWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, usernames)
	xsCount := len(usernames)

	// Test lg size (should be larger)
	config.Size = "lg"
	usernames, err = loadUsernameWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, usernames)
	lgCount := len(usernames)

	// Verify size relationships
	assert.True(t, xsCount <= defaultCount, "XS wordlist should be smaller than or equal to default")
	assert.True(t, defaultCount <= lgCount, "Default wordlist should be smaller than or equal to large")
}

func TestLoadPasswordWordlistSizes(t *testing.T) {
	config := AuthBruteforceConfig{
		Mode:   "embedded",
		Format: "separate",
	}

	// Test default size
	config.Size = "default"
	passwords, err := loadPasswordWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, passwords)
	defaultCount := len(passwords)

	// Test xs size (should be smaller)
	config.Size = "xs"
	passwords, err = loadPasswordWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, passwords)
	xsCount := len(passwords)

	// Test lg size (should be larger)
	config.Size = "lg"
	passwords, err = loadPasswordWordlist(config)
	assert.NoError(t, err)
	assert.NotEmpty(t, passwords)
	lgCount := len(passwords)

	// Verify size relationships
	assert.True(t, xsCount <= defaultCount, "XS wordlist should be smaller than or equal to default")
	assert.True(t, defaultCount <= lgCount, "Default wordlist should be smaller than or equal to large")
}

func TestLoadUserPasswordPairs(t *testing.T) {
	config := AuthBruteforceConfig{
		Mode:   "embedded",
		Format: "pairs",
		Size:   "default",
	}

	usernames, passwords, err := loadUserPasswordPairs(config)

	assert.NoError(t, err)
	assert.NotEmpty(t, usernames)
	assert.NotEmpty(t, passwords)
	assert.Equal(t, len(usernames), len(passwords), "Should have same number of usernames and passwords")

	// Check that we have some common pairs
	found := false
	for i, username := range usernames {
		if username == "admin" && passwords[i] == "admin" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should contain admin:admin pair")
}

func TestGetConfiguredWordlistSize(t *testing.T) {
	// Test with explicit size
	size := getConfiguredWordlistSize("usernames", "lg")
	assert.Equal(t, "lg", size)

	// Test with invalid size (should fallback to default)
	size = getConfiguredWordlistSize("usernames", "invalid")
	assert.Equal(t, "default", size)

	// Test with empty size (should fallback to default)
	size = getConfiguredWordlistSize("usernames", "")
	assert.Equal(t, "default", size)
}

func TestAuthBruteforceWithUserPasswordPairs(t *testing.T) {
	// Create a test server that accepts specific credentials
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok && username == "admin" && password == "admin" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		} else {
			w.Header().Set("WWW-Authenticate", `Basic realm="Test Realm"`)
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

	// Create a history item
	historyItem := &db.History{
		Method:      "GET",
		URL:         server.URL + "/protected",
		RawRequest:  rawRequest,
		WorkspaceID: func() *uint { id := uint(1); return &id }(),
		TaskID:      func() *uint { id := uint(1); return &id }(),
	}

	// Configure bruteforce with user-password pairs
	config := AuthBruteforceConfig{
		AuthType:      AuthTypeBasic,
		Mode:          "embedded",
		Format:        "pairs",
		Size:          "default",
		Concurrency:   2,
		MaxAttempts:   50,
		StopOnSuccess: true,
	}

	// Run bruteforce
	result := BruteforceAuth(historyItem, `Basic realm="Test Realm"`, config)

	// Assertions
	assert.NotNil(t, result, "Result should not be nil")
	assert.True(t, result.Found, "Should find valid credentials")
	assert.Equal(t, "admin", result.Username, "Should find correct username")
	assert.Equal(t, "admin", result.Password, "Should find correct password")
	assert.Equal(t, http.StatusOK, result.StatusCode, "Should return success status")
	assert.Greater(t, result.Attempts, 0, "Should have made some attempts")
	assert.Greater(t, result.Duration, time.Duration(0), "Should have taken some time")
}

// Helper function to validate digest auth
func validateDigestAuth(authHeader, expectedUsername, expectedPassword, method, uri string) bool {
	if authHeader == "" {
		return false
	}

	// Parse the digest auth header
	params := ParseDigestParams(authHeader)

	// Get the provided username
	username, ok := params["username"]
	if !ok {
		return false
	}

	// Check if username matches
	if username != expectedUsername {
		return false
	}

	// Get required digest parameters
	realm, hasRealm := params["realm"]
	nonce, hasNonce := params["nonce"]
	response, hasResponse := params["response"]
	digestUri, hasUri := params["uri"]

	if !hasRealm || !hasNonce || !hasResponse || !hasUri {
		return false
	}

	// Calculate the expected response hash
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

	// Compare the response hashes
	return response == expectedResponse
}
