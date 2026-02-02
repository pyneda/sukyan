package openapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// AuthenticationEnforcementAudit tests if endpoints that require authentication
// according to the OpenAPI spec actually enforce it.
type AuthenticationEnforcementAudit struct {
	Options         *OpenAPIAuditOptions
	Definition      *db.APIDefinition
	Endpoint        *db.APIEndpoint
	BaseHistory     *db.History
	SecuritySchemes []*db.APIDefinitionSecurityScheme
}

// Run executes the authentication enforcement audit
func (a *AuthenticationEnforcementAudit) Run() {
	auditLog := log.With().
		Str("audit", "openapi-auth-enforcement").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping authentication enforcement audit")
			return
		default:
		}
	}

	// Check if the operation has security requirements
	if a.Options.Operation == nil || len(a.Options.Operation.Security) == 0 {
		auditLog.Debug().Msg("No security requirements defined for this operation, skipping")
		return
	}

	// Fetch security schemes for this definition
	schemes, err := db.Connection().GetAPIDefinitionSecuritySchemes(a.Definition.ID)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to fetch security schemes")
		return
	}
	a.SecuritySchemes = schemes

	// Build a map for quick lookup
	schemeMap := make(map[string]*db.APIDefinitionSecurityScheme)
	for _, scheme := range schemes {
		schemeMap[scheme.Name] = scheme
	}

	// Check if we have scheme definitions for the required security
	var applicableSchemes []*db.APIDefinitionSecurityScheme
	for _, secReq := range a.Options.Operation.Security {
		if scheme, ok := schemeMap[secReq.Name]; ok {
			applicableSchemes = append(applicableSchemes, scheme)
		}
	}

	if len(applicableSchemes) == 0 {
		auditLog.Debug().Msg("No matching security scheme definitions found for operation requirements")
		return
	}

	auditLog.Info().
		Str("path", a.Endpoint.Path).
		Str("method", a.Endpoint.Method).
		Int("security_requirements", len(a.Options.Operation.Security)).
		Int("applicable_schemes", len(applicableSchemes)).
		Msg("Starting authentication enforcement audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	// Test without authentication based on the actual security schemes
	a.testWithoutAuthentication(client, applicableSchemes)

	auditLog.Info().Msg("Completed authentication enforcement audit")
}

// testWithoutAuthentication sends a request with auth removed based on actual security schemes
func (a *AuthenticationEnforcementAudit) testWithoutAuthentication(client *http.Client, schemes []*db.APIDefinitionSecurityScheme) {
	req, err := http_utils.BuildRequestFromHistoryItem(a.BaseHistory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to build request from history")
		return
	}

	req = req.WithContext(a.Options.Ctx)

	// Remove authentication based on actual security scheme definitions
	var removedAuth []string
	for _, scheme := range schemes {
		switch scheme.Type {
		case "apiKey":
			switch scheme.In {
			case "header":
				if scheme.ParameterName != "" {
					req.Header.Del(scheme.ParameterName)
					removedAuth = append(removedAuth, fmt.Sprintf("Header: %s", scheme.ParameterName))
				}
			case "query":
				if scheme.ParameterName != "" {
					q := req.URL.Query()
					q.Del(scheme.ParameterName)
					req.URL.RawQuery = q.Encode()
					removedAuth = append(removedAuth, fmt.Sprintf("Query: %s", scheme.ParameterName))
				}
			case "cookie":
				if scheme.ParameterName != "" {
					// Remove specific cookie - rebuild cookie header without the target cookie
					a.removeCookie(req, scheme.ParameterName)
					removedAuth = append(removedAuth, fmt.Sprintf("Cookie: %s", scheme.ParameterName))
				}
			}
		case "http":
			// HTTP authentication uses Authorization header
			req.Header.Del("Authorization")
			removedAuth = append(removedAuth, fmt.Sprintf("Authorization header (%s)", scheme.Scheme))
		case "oauth2", "openIdConnect":
			// OAuth2 and OpenID Connect typically use Bearer token in Authorization header
			req.Header.Del("Authorization")
			removedAuth = append(removedAuth, "Authorization header (Bearer token)")
		}
	}

	if len(removedAuth) == 0 {
		log.Debug().Msg("No authentication credentials identified to remove")
		return
	}

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})

	if result.Err != nil || result.History == nil {
		return
	}

	// Check if the response looks like a successful/authenticated response
	if a.isSuccessfulResponse(result.History) {
		a.reportVulnerability(result.History, removedAuth)
	}
}

// removeCookie removes a specific cookie from the request
func (a *AuthenticationEnforcementAudit) removeCookie(req *http.Request, cookieName string) {
	cookies := req.Cookies()
	req.Header.Del("Cookie")
	for _, cookie := range cookies {
		if cookie.Name != cookieName {
			req.AddCookie(cookie)
		}
	}
}

// isSuccessfulResponse checks if the response indicates successful access
// rather than an authentication error
func (a *AuthenticationEnforcementAudit) isSuccessfulResponse(history *db.History) bool {
	// First, check against known unauthenticated fingerprints if we have behavior data
	if a.Options.BehaviorResult != nil {
		body, _ := history.ResponseBody()
		bodyHash := fmt.Sprintf("%x", sha256.Sum256(body))

		if a.Options.BehaviorResult.MatchesUnauthenticated(history.StatusCode, bodyHash, len(body)) {
			// Response matches known unauthenticated pattern - NOT vulnerable
			return false
		}
	}

	// Check status code - 401/403 indicate auth is enforced
	if history.StatusCode == 401 || history.StatusCode == 403 {
		return false
	}

	// 4xx generally indicates rejection (might be other validation errors)
	if history.StatusCode >= 400 && history.StatusCode < 500 {
		// Check response body for auth-related messages
		body, err := history.ResponseBody()
		if err == nil {
			bodyLower := strings.ToLower(string(body))
			authIndicators := []string{
				"unauthorized", "unauthenticated", "authentication required",
				"not authenticated", "login required", "access denied",
				"invalid token", "missing token", "token required",
				"api key required", "invalid api key", "forbidden",
				"credentials required", "please authenticate",
			}
			for _, indicator := range authIndicators {
				if strings.Contains(bodyLower, indicator) {
					return false
				}
			}
		}
	}

	// 2xx responses without auth could be vulnerable
	if history.StatusCode >= 200 && history.StatusCode < 300 {
		// Extra validation: check if response looks like an error wrapped in 200
		body, err := history.ResponseBody()
		if err == nil {
			bodyLower := strings.ToLower(string(body))

			// Check for error indicators that some APIs return with 200
			errorIndicators := []string{
				"\"error\":", "\"errors\":", "\"status\":\"error\"",
				"\"success\":false", "\"authenticated\":false",
				"\"authorized\":false", "\"message\":\"unauthorized\"",
			}
			for _, indicator := range errorIndicators {
				if strings.Contains(bodyLower, indicator) {
					return false
				}
			}
		}

		return true
	}

	// 3xx redirects might indicate redirection to login page
	if history.StatusCode >= 300 && history.StatusCode < 400 {
		headers, _ := history.ResponseHeaders()
		if locations, ok := headers["Location"]; ok && len(locations) > 0 {
			location := strings.ToLower(locations[0])
			if strings.Contains(location, "login") || strings.Contains(location, "auth") ||
				strings.Contains(location, "signin") || strings.Contains(location, "sso") {
				return false
			}
		}
	}

	return false
}

// reportVulnerability creates the issue with technical details
func (a *AuthenticationEnforcementAudit) reportVulnerability(history *db.History, removedAuth []string) {
	// Build security requirements description
	var secReqs []string
	for _, req := range a.Options.Operation.Security {
		scopeInfo := ""
		if len(req.Scopes) > 0 {
			scopeInfo = fmt.Sprintf(" (scopes: %s)", strings.Join(req.Scopes, ", "))
		}
		secReqs = append(secReqs, fmt.Sprintf("- %s (%s)%s", req.Name, req.Type, scopeInfo))
	}

	// Build security scheme details
	var schemeDetails []string
	for _, scheme := range a.SecuritySchemes {
		detail := fmt.Sprintf("- %s: type=%s", scheme.Name, scheme.Type)
		if scheme.In != "" {
			detail += fmt.Sprintf(", in=%s", scheme.In)
		}
		if scheme.ParameterName != "" {
			detail += fmt.Sprintf(", name=%s", scheme.ParameterName)
		}
		if scheme.Scheme != "" {
			detail += fmt.Sprintf(", scheme=%s", scheme.Scheme)
		}
		schemeDetails = append(schemeDetails, detail)
	}

	details := fmt.Sprintf(`The endpoint accepted a request without authentication credentials.

Endpoint: %s %s
Operation ID: %s

OpenAPI Security Requirements:
%s

Security Scheme Definitions:
%s

Removed Authentication:
%s

Test Details:
- Response Status: %d
- Response Size: %d bytes

The server returned a successful response despite the OpenAPI specification
declaring that authentication is required for this endpoint.`,
		a.Endpoint.Method,
		a.Endpoint.Path,
		a.Endpoint.OperationID,
		strings.Join(secReqs, "\n"),
		strings.Join(schemeDetails, "\n"),
		"- "+strings.Join(removedAuth, "\n- "),
		history.StatusCode,
		history.ResponseBodySize,
	)

	reportIssue(history, db.ApiAuthenticationNotEnforcedCode, details, 85, a.Options)
}
