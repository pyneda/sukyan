package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/discovery"
	"github.com/pyneda/sukyan/pkg/graphql"
)

type apiDefsAuthParams struct {
	AuthType    string
	Username    string
	Password    string
	Token       string
	APIKeyName  string
	APIKeyValue string
	APIKeyIn    string
}

func fetchAPIDefinitionContent(specURL string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(specURL)
	if err != nil {
		return nil, fmt.Errorf("fetching API definition from %s: %w", specURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status code %d from %s", resp.StatusCode, specURL)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
}

func loadAPIDefinitionContent(url, filePath string) (content []byte, sourceURL string, err error) {
	if url != "" {
		sourceURL = url
		content, err = fetchAPIDefinitionContent(url)
		if err != nil {
			return nil, "", err
		}
		return content, sourceURL, nil
	}

	sourceURL = "file://" + filePath
	content, err = os.ReadFile(filePath)
	if err != nil {
		return nil, "", err
	}
	return content, sourceURL, nil
}

func detectAPIDefinitionType(content []byte, sourceURL string) db.APIDefinitionType {
	return pkgapi.DetectAPIType(content, sourceURL)
}

// looksLikeIntrospectionResult reports whether content already carries a GraphQL
// schema, in either the `{"data":{"__schema":…}}` or bare `{"__schema":…}` shape.
func looksLikeIntrospectionResult(content []byte) bool {
	var payload struct {
		Data *struct {
			Schema json.RawMessage `json:"__schema"`
		} `json:"data"`
		Schema json.RawMessage `json:"__schema"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return false
	}
	if payload.Data != nil && len(payload.Data.Schema) > 0 {
		return true
	}
	return len(payload.Schema) > 0
}

// fetchGraphQLIntrospection retrieves a schema by POSTing the introspection
// query. A GraphQL endpoint answers GET with its IDE's HTML page (GraphiQL,
// Apollo Sandbox, Playground), so a plain fetch of the endpoint URL never yields
// a schema.
func fetchGraphQLIntrospection(endpointURL string) ([]byte, error) {
	body, err := json.Marshal(map[string]string{"query": graphql.IntrospectionQuery})
	if err != nil {
		return nil, fmt.Errorf("marshalling introspection query: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building introspection request for %s: %w", endpointURL, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspecting %s: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading introspection response from %s: %w", endpointURL, err)
	}
	if !looksLikeIntrospectionResult(content) {
		return nil, fmt.Errorf("introspection on %s returned no schema (status %d); it is likely disabled", endpointURL, resp.StatusCode)
	}
	return content, nil
}

// resolveGraphQLDefinitionContent upgrades whatever a GET returned into a real
// introspection result when the target is a live GraphQL endpoint. Content that
// already holds a schema (a saved introspection file, for example) is returned
// untouched.
func resolveGraphQLDefinitionContent(content []byte, apiType db.APIDefinitionType, url string) ([]byte, error) {
	if apiType != db.APIDefinitionTypeGraphQL || url == "" || looksLikeIntrospectionResult(content) {
		return content, nil
	}
	return fetchGraphQLIntrospection(url)
}

func createAuthConfig(workspaceID uint, params apiDefsAuthParams) (*db.APIAuthConfig, error) {
	var authType db.APIAuthType
	switch params.AuthType {
	case "basic":
		authType = db.APIAuthTypeBasic
	case "bearer":
		authType = db.APIAuthTypeBearer
	case "api_key":
		authType = db.APIAuthTypeAPIKey
	default:
		return nil, nil
	}

	var apiKeyLocation db.APIKeyLocation
	switch params.APIKeyIn {
	case "query":
		apiKeyLocation = db.APIKeyLocationQuery
	case "cookie":
		apiKeyLocation = db.APIKeyLocationCookie
	default:
		apiKeyLocation = db.APIKeyLocationHeader
	}

	config := &db.APIAuthConfig{
		WorkspaceID:    workspaceID,
		Name:           "CLI Auth - " + time.Now().Format("2006-01-02 15:04:05"),
		Type:           authType,
		Username:       params.Username,
		Password:       params.Password,
		Token:          params.Token,
		TokenPrefix:    "Bearer",
		APIKeyName:     params.APIKeyName,
		APIKeyValue:    params.APIKeyValue,
		APIKeyLocation: apiKeyLocation,
	}

	return db.Connection().CreateAPIAuthConfig(config)
}

func parseAndPersistDefinition(content []byte, sourceURL string, apiType db.APIDefinitionType, workspaceID uint, authConfig *db.APIAuthConfig) (*db.APIDefinition, error) {
	opts := discovery.APIPersistenceFromContentOptions{
		WorkspaceID: workspaceID,
		SourceURL:   sourceURL,
	}
	if authConfig != nil {
		opts.AuthConfigID = &authConfig.ID
	}
	return discovery.PersistAPIDefinitionFromContent(content, apiType, opts)
}
