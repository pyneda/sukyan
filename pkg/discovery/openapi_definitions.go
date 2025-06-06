package discovery

import (
	"encoding/json"
	"math"
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

func IsOpenAPIValidationFunc(history *db.History) (bool, string, int) {
	confidence := 20
	details := make([]string, 0)

	if history.StatusCode != 200 {
		confidence = 15
	}

	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "application/json") {
		confidence += 20
		if strings.HasSuffix(history.URL, "json") {
			confidence += 10
		}
		details = append(details, "JSON content type detected")
	} else if strings.Contains(contentType, "application/yaml") || strings.Contains(contentType, "application/x-yaml") {
		confidence += 30
		details = append(details, "YAML content type detected")
	} else if strings.Contains(contentType, "text/html") {
		confidence -= 20
	}

	body, _ := history.ResponseBody()
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	commonFields := []string{
		"\"swagger\":", "\"openapi\":", "\"info\":", "\"paths\":", "\"components\":",
		"\"definitions\":", "\"consumes\":", "\"produces\":", "$ref", "\"responses\":",
		"\"servers\":", "\"security\":", "\"tags\":", "\"externalDocs\":",
		"\"parameters\":", "\"schemas\":", "\"requestBody\":", "\"callbacks\":",
		"\"links\":", "\"headers\":", "\"securitySchemes\":", "\"examples\":",
		"\"description\":", "\"summary\":", "\"operationId\":", "\"deprecated\":",
		"\"content\":", "\"mediaType\":", "\"required\":", "\"type\":", "\"properties\":",
		"swagger:", "openapi:", "info:", "paths:", "components:", "definitions:",
		"servers:", "security:", "tags:", "externalDocs:", "parameters:", "schemas:",
		"requestBody:", "callbacks:", "links:", "headers:", "securitySchemes:",
		"examples:", "description:", "summary:", "operationId:", "deprecated:",
		"content:", "mediaType:", "required:", "type:", "properties:",
	}

	fieldCount := 0
	matchedFields := []string{}

	for _, field := range commonFields {
		if strings.Contains(bodyLower, field) {
			fieldCount++
			matchedFields = append(matchedFields, field)
		}
	}

	// Increment confidence by 15 per match, with a maximum of 100
	confidenceIncrement := 15
	confidence = int(math.Min(float64(confidence+(confidenceIncrement*fieldCount)), 100))

	if fieldCount >= 2 {
		details = append(details, "Multiple OpenAPI/Swagger fields detected: "+strings.Join(matchedFields, ", "))
	}

	headersStr, err := history.GetResponseHeadersAsString()
	if err == nil {
		headersLower := strings.ToLower(headersStr)
		if strings.Contains(headersLower, "swagger") || strings.Contains(headersLower, "openapi") {
			confidence = int(math.Min(float64(confidence+20), 100))
			details = append(details, "API documentation related header detected")
		}
	}

	var jsonObj map[string]interface{}

	if json.Unmarshal(body, &jsonObj) == nil {
		if _, hasSwagger := jsonObj["swagger"]; hasSwagger {
			confidence += 20
			details = append(details, "Valid Swagger version field found")
		}
		if _, hasOpenAPI := jsonObj["openapi"]; hasOpenAPI {
			confidence += 20
			details = append(details, "Valid OpenAPI version field found")
		}
	}

	if confidence > 50 {
		if confidence > 100 {
			confidence = 100
		}
		return true, strings.Join(details, "\n"), confidence
	}

	return false, "", 0
}

func DiscoverOpenapiDefinitions(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
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
}
