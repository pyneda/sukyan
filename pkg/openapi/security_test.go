package openapi

import (
	"reflect"
	"strings"
	"testing"
)

const securitySpec = `{
  "openapi": "3.0.0",
  "info": {"title": "x", "version": "1"},
  "security": [{"bearerAuth": []}],
  "paths": {
    "/inherits": {"get": {"responses": {"200": {"description": "ok"}}}},
    "/public": {"get": {"security": [], "responses": {"200": {"description": "ok"}}}},
    "/optional": {"get": {"security": [{}, {"bearerAuth": []}], "responses": {"200": {"description": "ok"}}}},
    "/alternatives": {"get": {"security": [{"bearerAuth": []}, {"basicAuth": []}], "responses": {"200": {"description": "ok"}}}},
    "/combined": {"get": {"security": [{"apiKeyHeader": [], "apiKeyQuery": []}], "responses": {"200": {"description": "ok"}}}},
    "/scoped": {"get": {"security": [{"oauth2": ["read:items", "write:items"]}], "responses": {"200": {"description": "ok"}}}},
    "/anonymous-key": {"get": {"security": [{"namelessKey": []}], "responses": {"200": {"description": "ok"}}}},
    "/mixed-case": {"get": {"security": [{"mixedCaseKey": []}], "responses": {"200": {"description": "ok"}}}},
    "/mtls": {"get": {"security": [{"mtls": []}], "responses": {"200": {"description": "ok"}}}}
  },
  "components": {
    "securitySchemes": {
      "bearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
      "basicAuth": {"type": "http", "scheme": "basic"},
      "apiKeyHeader": {"type": "apiKey", "name": "X-API-Key", "in": "header"},
      "apiKeyQuery": {"type": "apiKey", "name": "api_key", "in": "query"},
      "oauth2": {"type": "oauth2", "flows": {"clientCredentials": {"tokenUrl": "https://example.com/token", "scopes": {}}}},
      "namelessKey": {"type": "apiKey", "in": "header"},
      "mixedCaseKey": {"type": "apiKey", "name": "X-Mixed", "in": "Header"},
      "mtls": {"type": "mutualTLS"}
    }
  }
}`

func securityEndpoints(t *testing.T) map[string]Endpoint {
	t.Helper()
	doc := mustParse(t, securitySpec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	return endpointsByPath(endpoints)
}

// TestOperationSecurityOverrides is the core of the auth model: an operation
// declaring "security: []" is explicitly public and must not receive the document's
// global credentials.
func TestOperationSecurityOverrides(t *testing.T) {
	byPath := securityEndpoints(t)

	inherits := requireEndpoint(t, byPath, "/inherits")
	if !inherits.RequiresAuth {
		t.Error("/inherits should inherit the global security requirement")
	}
	if got := inherits.Requests[0].Headers["Authorization"]; got != "Bearer <TOKEN>" {
		t.Errorf("/inherits Authorization = %q, want Bearer <TOKEN>", got)
	}

	public := requireEndpoint(t, byPath, "/public")
	if public.RequiresAuth {
		t.Error("/public declares security: [] and must not require auth")
	}
	if len(public.Security) != 0 {
		t.Errorf("/public Security = %+v, want empty", public.Security)
	}
	for _, request := range public.Requests {
		if _, ok := request.Headers["Authorization"]; ok {
			t.Errorf("/public must not carry credentials, got %v", request.Headers)
		}
	}
}

// TestOptionalSecurityAlternative covers the empty requirement object, which is how
// a spec says authentication is optional. Dropping it silently turns an anonymously
// reachable endpoint into one the scanner believes is protected.
func TestOptionalSecurityAlternative(t *testing.T) {
	byPath := securityEndpoints(t)
	optional := requireEndpoint(t, byPath, "/optional")

	if optional.RequiresAuth {
		t.Error("/optional offers an anonymous alternative and must not be reported as requiring auth")
	}
	if len(optional.Security) != 2 {
		t.Fatalf("/optional Security = %d alternatives, want 2 (anonymous and bearer)", len(optional.Security))
	}
	if len(optional.Security[0].Schemes) != 0 {
		t.Errorf("first alternative should be the anonymous one, got %+v", optional.Security[0].Schemes)
	}
}

// TestSecurityAlternativesAreNotCombined guards against attaching every OR
// alternative's credentials to a single request, which no real client would send.
func TestSecurityAlternativesAreNotCombined(t *testing.T) {
	byPath := securityEndpoints(t)
	alternatives := requireEndpoint(t, byPath, "/alternatives")

	if got := len(alternatives.Security); got != 2 {
		t.Errorf("Security = %d alternatives, want 2", got)
	}
	if got := alternatives.Requests[0].Headers["Authorization"]; got != "Bearer <TOKEN>" {
		t.Errorf("Authorization = %q, want only the first alternative (Bearer <TOKEN>)", got)
	}
}

// TestCombinedSecuritySchemes covers the AND relationship: two schemes inside one
// requirement must both be applied.
func TestCombinedSecuritySchemes(t *testing.T) {
	byPath := securityEndpoints(t)
	combined := requireEndpoint(t, byPath, "/combined")

	request := combined.Requests[0]
	if got := request.Headers["X-API-Key"]; got != "<API_KEY>" {
		t.Errorf("X-API-Key = %q, want <API_KEY>", got)
	}
	if !strings.Contains(request.URL, "api_key=") {
		t.Errorf("URL %q should carry the query API key", request.URL)
	}
	if got := len(combined.Security[0].Schemes); got != 2 {
		t.Errorf("requirement holds %d schemes, want 2 combined with AND", got)
	}
}

func TestSecurityScopesArePreserved(t *testing.T) {
	byPath := securityEndpoints(t)
	scoped := requireEndpoint(t, byPath, "/scoped")

	if len(scoped.Security) != 1 || len(scoped.Security[0].Schemes) != 1 {
		t.Fatalf("unexpected security shape: %+v", scoped.Security)
	}
	want := []string{"read:items", "write:items"}
	if got := scoped.Security[0].Schemes[0].Scopes; !reflect.DeepEqual(got, want) {
		t.Errorf("scopes = %v, want %v", got, want)
	}
	if got := scoped.Requests[0].Headers["Authorization"]; got != "Bearer <ACCESS_TOKEN>" {
		t.Errorf("oauth2 Authorization = %q, want Bearer <ACCESS_TOKEN>", got)
	}
}

func TestMalformedSecuritySchemesDoNotFabricateCredentials(t *testing.T) {
	byPath := securityEndpoints(t)

	// An apiKey scheme without a name is malformed; inventing a header would send a
	// credential the API never reads and mislabel where auth lives.
	nameless := requireEndpoint(t, byPath, "/anonymous-key")
	for _, request := range nameless.Requests {
		if _, ok := request.Headers["X-API-Key"]; ok {
			t.Error("a nameless apiKey scheme must not fabricate an X-API-Key header")
		}
		if strings.Contains(request.URL, "X-API-Key") {
			t.Errorf("a nameless apiKey scheme must not fabricate a query parameter: %s", request.URL)
		}
	}

	// mutualTLS is a transport-level scheme; there is no header to send.
	mtls := requireEndpoint(t, byPath, "/mtls")
	for _, request := range mtls.Requests {
		if _, ok := request.Headers["X-Auth-Note"]; ok {
			t.Error("mutualTLS must not add a fabricated header to the request")
		}
	}
}

func TestAPIKeyLocationIsCaseInsensitive(t *testing.T) {
	byPath := securityEndpoints(t)
	mixed := requireEndpoint(t, byPath, "/mixed-case")

	if got := mixed.Requests[0].Headers["X-Mixed"]; got != "<API_KEY>" {
		t.Errorf(`in: "Header" should be honoured; X-Mixed = %q`, got)
	}
}

func TestGetSecuritySchemesIsSortedAndComplete(t *testing.T) {
	doc := mustParse(t, securitySpec)
	schemes := doc.GetSecuritySchemes()

	if len(schemes) != 8 {
		t.Fatalf("schemes = %d, want 8", len(schemes))
	}
	for i := 1; i < len(schemes); i++ {
		if schemes[i-1].Name > schemes[i].Name {
			t.Fatalf("schemes are not sorted by name: %q before %q", schemes[i-1].Name, schemes[i].Name)
		}
	}

	byName := make(map[string]SecurityScheme, len(schemes))
	for _, scheme := range schemes {
		byName[scheme.Name] = scheme
	}
	if got := byName["bearerAuth"].BearerFormat; got != "JWT" {
		t.Errorf("bearerAuth BearerFormat = %q, want JWT", got)
	}
	if got := byName["apiKeyQuery"]; got.In != "query" || got.ParameterName != "api_key" {
		t.Errorf("apiKeyQuery = %+v, want in=query name=api_key", got)
	}
}

// TestSecuritySchemesFeedTheAuthEnforcementAudit pins the scheme shape that
// pkg/active/api/openapi/authentication_enforcement.go switches on when it strips
// credentials. A type it has no case for makes that audit silently do nothing.
func TestSecuritySchemesFeedTheAuthEnforcementAudit(t *testing.T) {
	recognised := map[string]bool{"apiKey": true, "http": true, "oauth2": true, "openIdConnect": true, "mutualTLS": true}

	for _, spec := range []string{securitySpec, swagger2Spec} {
		doc := mustParse(t, spec)
		for _, scheme := range doc.GetSecuritySchemes() {
			if !recognised[scheme.Type] {
				t.Errorf("scheme %q has type %q, which the auth enforcement audit cannot strip", scheme.Name, scheme.Type)
			}
			if scheme.Type == "apiKey" && scheme.In == "" {
				t.Errorf("apiKey scheme %q has no location; the audit cannot remove it", scheme.Name)
			}
		}
	}
}

func TestGetOperationSecurityRequirementsDistinguishesOverride(t *testing.T) {
	doc := mustParse(t, securitySpec)

	for _, entry := range doc.Operations() {
		requirements, overridden := doc.GetOperationSecurityRequirements(entry.Operation)

		switch entry.Path {
		case "/inherits":
			if overridden {
				t.Error("/inherits declares no security and must not report an override")
			}
		case "/public":
			if !overridden {
				t.Error("/public declares security: [] and must report an override")
			}
			if len(requirements) != 0 {
				t.Errorf("/public requirements = %+v, want none", requirements)
			}
		}
	}
}

func TestGetGlobalSecurityIsSortedAndDeduplicated(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "security": [{"zeta": [], "alpha": []}, {"alpha": []}],
	  "paths": {},
	  "components": {"securitySchemes": {
	    "alpha": {"type": "http", "scheme": "bearer"},
	    "zeta": {"type": "http", "scheme": "basic"}}}
	}`

	doc := mustParse(t, spec)
	want := []string{"alpha", "zeta"}
	if got := doc.GetGlobalSecurity(); !reflect.DeepEqual(got, want) {
		t.Errorf("GetGlobalSecurity() = %v, want %v", got, want)
	}

	requirements := doc.GetGlobalSecurityRequirements()
	if len(requirements) != 2 {
		t.Fatalf("requirements = %d, want 2", len(requirements))
	}
	if got := requirements[0].Schemes[0].Name; got != "alpha" {
		t.Errorf("schemes within a requirement should be sorted, got %q first", got)
	}
}
