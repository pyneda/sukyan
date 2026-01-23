package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var OAuthPaths = []string{
	// Standard OAuth and OIDC well-known paths
	".well-known/oauth-authorization-server",
	".well-known/openid-configuration",
	".well-known/jwks.json",

	// OAuth 2.0 standard endpoints
	"oauth/authorize",
	"oauth/token",
	"oauth2/authorize",
	"oauth2/token",

	// OpenID Connect paths
	"connect/authorize",
	"connect/token",

	// Keycloak specific
	"auth/realms",

	// Versioned endpoints
	"oauth/v2/authorize",
	"oauth/v2/token",
	"v1/oauth2/authorize",
	"v1/oauth2/token",

	// Additional well-known configurations
	"oauth/.well-known/openid-configuration",
	"auth/.well-known/openid-configuration",
	"identity/.well-known/openid-configuration",

	// JWKS endpoints
	"oauth/discovery/keys",
	"oauth/jwks.json",

	// Additional common vendor paths
	"realms/protocol/openid-connect",
	"oidc/.well-known/openid-configuration",
	"oidc/authorize",
	"oidc/token",

	// Azure AD specific
	"tenant/oauth2/authorize",
	"tenant/oauth2/token",

	// Auth0 specific
	"authorize",
	"oauth/userinfo",

	// Common additional endpoints
	"oauth/introspect",
	"oauth2/introspect",
	"oauth/revoke",
	"oauth2/revoke",
}

func isOAuthEndpointValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, _ := history.ResponseBody()
	bodyStr := string(body)
	details := fmt.Sprintf("OAuth endpoint found: %s\n", history.URL)
	confidence := 0

	var jsonData map[string]interface{}
	isJSON := json.Unmarshal([]byte(bodyStr), &jsonData) == nil

	oauthIndicators := []string{
		"authorization_endpoint",
		"token_endpoint",
		"issuer",
		"jwks_uri",
		"response_types_supported",
		"subject_types_supported",
		"id_token_signing_alg_values_supported",
		"grant_types_supported",
		"client_id",
		"scopes_supported",
		"claims_supported",
	}

	if isJSON {
		confidence += 10
		for _, indicator := range oauthIndicators {
			if _, exists := jsonData[indicator]; exists {
				confidence += 10
				details += fmt.Sprintf("- Contains OAuth/OpenID configuration: %s\n", indicator)
			}
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	if location, exists := headers["Location"]; exists {
		locationStr := strings.Join(location, " ")
		if strings.Contains(strings.ToLower(locationStr), "oauth") ||
			strings.Contains(strings.ToLower(locationStr), "authorize") {
			confidence += 10
			details += "- OAuth-related redirect detected\n"
		}
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 20 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverOAuthEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       OAuthPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: isOAuthEndpointValidationFunc,
		IssueCode:      db.OauthEndpointDetectedCode,
	})
}
