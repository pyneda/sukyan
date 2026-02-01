package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var OpenAPIPaths = []string{
	"api-docs.json",
	"api-docs.yaml",
	"api-docs.yml",
	"api/openapi.json",
	"api/openapi.yaml",
	"api/openapi.yml",
	"api/swagger.json",
	"docs/api-docs.json",
	"docs/api-docs.yaml",
	"docs/api-docs.yml",
	"docs/openapi.json",
	"docs/openapi.yaml",
	"docs/openapi.yml",
	"docs/swagger.json",
	"docs/swagger.yaml",
	"docs/swagger.yml",
	"swagger/properties.json",
	"swagger/properties.yaml",
	"swagger/docs.json",
	"swagger/docs.yaml",
	"openapi.json",
	"openapi.yaml",
	"openapi.yml",
	"swagger.json",
	"swagger.yaml",
	"swagger.yml",
	"api-spec.json",
	"api-spec.yaml",
	"api-spec.yml",
	"v1/openapi.json",
	"v1/swagger.json",
	"v2/openapi.json",
	"v2/swagger.json",
	"v3/openapi.json",
	"v3/swagger.json",
	"v1/api-docs.json",
	"v2/api-docs.json",
	"v3/api-docs.json",
	"api/v1/swagger.json",
	"api/v2/swagger.json",
	"api/v3/swagger.json",
	"documentation/openapi.json",
	"documentation/swagger.json",
	"api/documentation/openapi.json",
	"api/documentation/swagger.json",
	"api-documentation/openapi.json",
	"api-documentation/swagger.json",
	"spec/openapi.json",
	"spec/swagger.json",
	"api/spec/openapi.json",
	"api/spec/swagger.json",
	"schema/openapi.json",
	"schema/swagger.json",
	"api/schema/openapi.json",
	"api/schema/swagger.json",
	"reference/openapi.json",
	"reference/swagger.json",
	"api/reference/openapi.json",
	"api/reference/swagger.json",
	"swagger-ui/swagger.json",
	"swagger-resources/swagger.json",
	"api/swagger-resources/swagger.json",
	"swagger-config.json",
	"api-definition.json",
	"api/definition/swagger.json",
}

func IsOpenAPIValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, _ := history.ResponseBody()
	if len(body) == 0 {
		return false, "", 0
	}

	var jsonObj map[string]interface{}
	if json.Unmarshal(body, &jsonObj) != nil {
		return false, "", 0
	}

	swaggerVersion, hasSwagger := jsonObj["swagger"]
	openapiVersion, hasOpenAPI := jsonObj["openapi"]

	if !hasSwagger && !hasOpenAPI {
		return false, "", 0
	}

	confidence := 40
	details := make([]string, 0)

	if hasSwagger {
		if v, ok := swaggerVersion.(string); ok && strings.HasPrefix(v, "2.") {
			confidence += 15
			details = append(details, fmt.Sprintf("Swagger version: %s", v))
		} else {
			details = append(details, "Swagger version field present")
		}
	}

	if hasOpenAPI {
		if v, ok := openapiVersion.(string); ok && (strings.HasPrefix(v, "3.0") || strings.HasPrefix(v, "3.1")) {
			confidence += 15
			details = append(details, fmt.Sprintf("OpenAPI version: %s", v))
		} else {
			details = append(details, "OpenAPI version field present")
		}
	}

	definitiveFields := map[string]int{
		"info": 10, "paths": 15, "components": 10, "definitions": 10,
		"servers": 5, "basePath": 5, "host": 5, "consumes": 5, "produces": 5,
	}

	matchedFields := []string{}
	for field, points := range definitiveFields {
		if _, exists := jsonObj[field]; exists {
			confidence += points
			matchedFields = append(matchedFields, field)
		}
	}

	if len(matchedFields) > 0 {
		details = append(details, "OpenAPI fields found: "+strings.Join(matchedFields, ", "))
	}

	if info, ok := jsonObj["info"].(map[string]interface{}); ok {
		if _, hasTitle := info["title"]; hasTitle {
			confidence += 5
		}
		if _, hasVersion := info["version"]; hasVersion {
			confidence += 5
		}
	}

	if paths, ok := jsonObj["paths"].(map[string]interface{}); ok && len(paths) > 0 {
		validPathCount := 0
		for pathKey := range paths {
			if strings.HasPrefix(pathKey, "/") {
				validPathCount++
			}
		}
		if validPathCount > 0 {
			confidence += 10
			details = append(details, fmt.Sprintf("Found %d API path definitions", validPathCount))
		}
	}

	if strings.Contains(strings.ToLower(history.ResponseContentType), "application/json") {
		confidence += 5
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 60 {
		return true, strings.Join(details, "\n"), confidence
	}

	return false, "", 0
}

func DiscoverOpenapiDefinitions(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	results, err := DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       OpenAPIPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "application/json, application/yaml, */*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsOpenAPIValidationFunc,
		IssueCode:      db.OpenapiDefinitionFoundCode,
	})

	persistDiscoveredAPIDefinitions(results, options, IsOpenAPIValidationFunc, PersistOpenAPIDefinition, "OpenAPI")

	return results, err
}
