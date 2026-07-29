package api

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/discovery"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

// ErrInvalidDefinition reports a document that could not be parsed as the API
// definition it claimed to be. It is the caller's input that is wrong, so import
// handlers answer it with a 400 rather than a 500.
var ErrInvalidDefinition = discovery.ErrInvalidDefinitionContent

type ImportOptions struct {
	WorkspaceID  uint
	Name         string
	SourceURL    string
	BaseURL      string
	Type         string
	AuthConfigID *uuid.UUID
}

type FetchedContent struct {
	Content   []byte
	SourceURL string
	Type      db.APIDefinitionType
}

func FetchAPIContent(url, content, typeHint string) (*FetchedContent, error) {
	result := &FetchedContent{}

	if url != "" {
		result.SourceURL = url
		if typeHint == "graphql" {
			parser := graphql.NewParser()
			bodyBytes, err := parser.FetchIntrospectionRaw(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch GraphQL schema from URL: %w", err)
			}
			result.Content = bodyBytes
		} else {
			bodyBytes, err := http_utils.FetchOpenAPISpec(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch API spec from URL: %w", err)
			}
			result.Content = bodyBytes
		}
	} else if content != "" {
		result.Content = []byte(content)
	} else {
		return nil, fmt.Errorf("either url or content is required")
	}

	result.Type = DetectAPIType(result.Content, result.SourceURL)
	if typeHint != "" {
		result.Type = db.APIDefinitionType(typeHint)
	}

	return result, nil
}

// ImportAPIDefinition stores a new definition parsed from content the caller
// already holds. It never replaces a definition that is already in the library —
// see discovery.PersistAPIDefinitionFromContent for why.
func ImportAPIDefinition(content []byte, sourceURL string, opts ImportOptions) (*db.APIDefinition, error) {
	apiType := db.APIDefinitionType(opts.Type)
	if apiType == "" {
		apiType = DetectAPIType(content, sourceURL)
	}
	contentOpts := discovery.APIPersistenceFromContentOptions{
		WorkspaceID:  opts.WorkspaceID,
		SourceURL:    sourceURL,
		Name:         opts.Name,
		BaseURL:      opts.BaseURL,
		AuthConfigID: opts.AuthConfigID,
	}
	return discovery.PersistAPIDefinitionFromContent(content, apiType, contentOpts)
}

func FindOperation(operations map[string]map[string]*openapi3.Operation, path, method string) *openapi3.Operation {
	if methods, ok := operations[path]; ok {
		if op, ok := methods[method]; ok {
			return op
		}
	}
	return nil
}

func DeriveBaseURLFromSpecURL(specURL string) string {
	if specURL == "" {
		return ""
	}

	lastSlash := strings.LastIndex(specURL, "/")
	if lastSlash == -1 {
		return specURL
	}

	afterSlash := specURL[lastSlash+1:]
	if strings.Contains(afterSlash, ".") {
		baseURL := specURL[:lastSlash]
		if strings.HasSuffix(baseURL, ":") {
			return specURL
		}
		return baseURL
	}

	return specURL
}
