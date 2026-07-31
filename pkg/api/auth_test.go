package api

import (
	"net/http"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://api.example.com/pets?a=1", nil)
	require.NoError(t, err)
	return req
}

func TestApplyAuthConfigBasic(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{Type: db.APIAuthTypeBasic, Username: "u", Password: "p"}, "")
	require.Equal(t, "Basic dTpw", req.Header.Get("Authorization"))
}

func TestApplyAuthConfigBearerUsesStoredTokenWhenNoDynamic(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{Type: db.APIAuthTypeBearer, Token: "stored"}, "")
	require.Equal(t, "Bearer stored", req.Header.Get("Authorization"))
}

func TestApplyAuthConfigBearerPrefersDynamicToken(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{Type: db.APIAuthTypeBearer, Token: "stored", TokenPrefix: "Token"}, "fresh")
	require.Equal(t, "Token fresh", req.Header.Get("Authorization"))
}

func TestApplyAuthConfigOAuth2(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{Type: db.APIAuthTypeOAuth2, Token: "oauth-token"}, "")
	require.Equal(t, "Bearer oauth-token", req.Header.Get("Authorization"))
}

func TestApplyAuthConfigAPIKeyInHeader(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{
		Type:           db.APIAuthTypeAPIKey,
		APIKeyName:     "X-Api-Key",
		APIKeyValue:    "secret",
		APIKeyLocation: db.APIKeyLocationHeader,
	}, "")
	require.Equal(t, "secret", req.Header.Get("X-Api-Key"))
}

func TestApplyAuthConfigAPIKeyInQuery(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{
		Type:           db.APIAuthTypeAPIKey,
		APIKeyName:     "api_key",
		APIKeyValue:    "secret",
		APIKeyLocation: db.APIKeyLocationQuery,
	}, "")
	require.Equal(t, "secret", req.URL.Query().Get("api_key"))
	require.Equal(t, "1", req.URL.Query().Get("a"))
}

func TestApplyAuthConfigCustomHeadersApplyToEveryType(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, &db.APIAuthConfig{
		Type:          db.APIAuthTypeBearer,
		Token:         "t",
		CustomHeaders: []db.APIAuthHeader{{HeaderName: "X-Tenant", HeaderValue: "acme"}},
	}, "")
	require.Equal(t, "acme", req.Header.Get("X-Tenant"))
	require.Equal(t, "Bearer t", req.Header.Get("Authorization"))
}

func TestApplyAuthConfigNilIsNoop(t *testing.T) {
	req := newTestRequest(t)
	ApplyAuthConfig(req, nil, "")
	require.Empty(t, req.Header.Get("Authorization"))
}
