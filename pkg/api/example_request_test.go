package api

import (
	"context"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/stretchr/testify/require"
)

func exampleOperation() *core.Operation {
	return &core.Operation{
		APIType: core.APITypeOpenAPI,
		Method:  "GET",
		Path:    "/pets",
		BaseURL: "https://api.example.com",
		Parameters: []core.Parameter{
			{Name: "limit", Location: core.ParameterLocationQuery, DataType: core.DataTypeInteger},
		},
	}
}

func TestBuildExampleRequestProducesRawHTTP(t *testing.T) {
	got, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, exampleOperation(), nil, false)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got.Raw, "GET /pets"), "raw was %q", got.Raw)
	require.Contains(t, got.Raw, "Host: api.example.com")
	require.Equal(t, "GET", got.Method)
	require.Contains(t, got.URL, "https://api.example.com/pets")
	require.Empty(t, got.Masked)
}

func TestBuildExampleRequestMasksBearerTokenByDefault(t *testing.T) {
	cfg := &db.APIAuthConfig{Type: db.APIAuthTypeBearer, Token: "super-secret-token"}

	got, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, exampleOperation(), cfg, false)
	require.NoError(t, err)
	require.NotContains(t, got.Raw, "super-secret-token")
	require.Contains(t, got.Raw, "Authorization: Bearer ****")
	require.Equal(t, []string{"Authorization"}, got.Masked)
}

func TestBuildExampleRequestRevealsWhenAsked(t *testing.T) {
	cfg := &db.APIAuthConfig{Type: db.APIAuthTypeBearer, Token: "super-secret-token"}

	got, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, exampleOperation(), cfg, true)
	require.NoError(t, err)
	require.Contains(t, got.Raw, "Bearer super-secret-token")
	require.Empty(t, got.Masked)
}

func TestBuildExampleRequestMasksAPIKeyInQuery(t *testing.T) {
	cfg := &db.APIAuthConfig{
		Type:           db.APIAuthTypeAPIKey,
		APIKeyName:     "api_key",
		APIKeyValue:    "secret-key-value",
		APIKeyLocation: db.APIKeyLocationQuery,
	}

	got, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, exampleOperation(), cfg, false)
	require.NoError(t, err)
	require.NotContains(t, got.Raw, "secret-key-value")
	require.NotContains(t, got.URL, "secret-key-value")
	require.Contains(t, got.Masked, "api_key")
}

func TestBuildExampleRequestMasksCustomHeaders(t *testing.T) {
	cfg := &db.APIAuthConfig{
		Type:          db.APIAuthTypeBearer,
		Token:         "t",
		CustomHeaders: []db.APIAuthHeader{{HeaderName: "X-Tenant-Secret", HeaderValue: "acme-private"}},
	}

	got, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, exampleOperation(), cfg, false)
	require.NoError(t, err)
	require.NotContains(t, got.Raw, "acme-private")
	require.Contains(t, got.Masked, "X-Tenant-Secret")
}

func TestBuildExampleRequestRejectsNilOperation(t *testing.T) {
	_, err := BuildExampleRequest(context.Background(), db.APIDefinitionTypeOpenAPI, nil, nil, false)
	require.Error(t, err)
}
