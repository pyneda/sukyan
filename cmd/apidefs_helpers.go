package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pyneda/sukyan/db"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	"github.com/pyneda/sukyan/pkg/discovery"
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
