package discovery

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var GraphQLPaths = []string{
	// Standard GraphQL endpoints
	"graphql",
	"api/graphql",
	"query",
	"api/query",
	"v1/graphql",
	"v2/graphql",
	"graphql/v1",
	"graphql/v2",
	"api/v1/graphql",
	"api/v2/graphql",
	"gql",
	"api/gql",

	// Development tools & playgrounds
	"graphiql",
	"playground",
	"explore",
	"graphql-playground",
	"graphql/playground",
	"api/graphql/playground",
	"graphql/explorer",
	"graphql-explorer",
	"graphql/console",
	"api/graphql/console",

	// Alternative paths & common variations
	"graphql/api",
	"graphql/schema",
	"schema",
	"api/schema",
	"graphql.php",
	"graphql.json",
	"graph",

	// Common backend framework paths
	"wp/graphql",
	"wp-json/graphql",
	"wp-json/wp/v2/graphql",

	// Special cases
	"_graphql",
	".graphql",
	"graphql-api",
	"api-graphql",
	"graphqlapi",
	"apigraphql",
}

var graphQLUIMarkers = []string{
	"graphiql",
	"altair",
	"graphql playground",
	"apollo studio",
	"prisma studio",
	"<title>graphql",
	"react-apollo",
	"graphql-playground",
}

type GraphQLValidationResponse struct {
	Data *struct {
		Schema struct {
			QueryType struct {
				Name string `json:"name"`
			} `json:"queryType"`
			Types []struct {
				Name string `json:"name"`
				Kind string `json:"kind"`
			} `json:"types"`
		} `json:"__schema"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func IsGraphQLValidationFunc(history *db.History) (bool, string, int) {
	confidence := 50
	details := make([]string, 0)

	if history.StatusCode != 200 {
		confidence = 30
	}

	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "application/json") {
		confidence += 10
		details = append(details, "JSON content type detected")
	} else if strings.Contains(contentType, "application/graphql") {
		confidence += 20
		details = append(details, "GraphQL content type detected")
	}

	if isGraphQLUI(history) {
		return true, "GraphQL Playground/UI detected", 95
	}

	var response GraphQLValidationResponse
	if err := json.Unmarshal(history.ResponseBody, &response); err == nil {
		if response.Data != nil && len(response.Data.Schema.Types) > 0 {
			confidence = 100
			details = append(details, "Valid GraphQL schema introspection response")
			return true, strings.Join(details, "\n"), confidence
		}

		if len(response.Errors) > 0 {
			confidence += 20
			for _, err := range response.Errors {
				if containsGraphQLErrorPattern(err.Message) {
					confidence = int(math.Min(float64(confidence+10), 100))
					details = append(details, fmt.Sprintf("GraphQL error detected: %s", err.Message))
				}
			}
		}
	}
	headersStr, err := history.GetResponseHeadersAsString()
	if err == nil {
		if strings.Contains(strings.ToLower(headersStr), "graphql") {
			confidence = int(math.Min(float64(confidence+30), 100))
			details = append(details, "GraphQL-related header detected")
		}
	}

	if confidence > 50 {
		return true, strings.Join(details, "\n"), confidence
	}

	return false, "", 0
}

func isGraphQLUI(history *db.History) bool {
	bodyStr := string(history.ResponseBody)
	bodyLower := strings.ToLower(bodyStr)

	if !strings.Contains(strings.ToLower(history.ResponseContentType), "text/html") {
		return false
	}

	markerCount := 0
	for _, marker := range graphQLUIMarkers {
		if strings.Contains(bodyLower, marker) {
			markerCount++
		}
	}

	return markerCount >= 2
}

func containsGraphQLErrorPattern(text string) bool {
	errorPatterns := []string{
		"must provide query string",
		"query not found",
		"syntax error",
		"graphql syntax error",
		"operation not found",
		"must provide an operation",
		"query is not valid",
		"cannot query field",
		"field does not exist",
		"directive not found",
	}

	textLower := strings.ToLower(text)
	for _, pattern := range errorPatterns {
		if strings.Contains(textLower, pattern) {
			return true
		}
	}

	return false
}

func DiscoverGraphQLEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	introspectionQuery := `{"query": "query { __schema { queryType { name } types { name kind } } }"}`
	// TODO: Another check for full schema introspection query, to parse it and generate requests to scan

	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "POST",
			Body:        introspectionQuery,
			Paths:       GraphQLPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsGraphQLValidationFunc,
		IssueCode:      db.GraphqlEndpointDetectedCode,
	})
}
