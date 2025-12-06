package openapi

import (
	"fmt"
	"strings"
	"testing"
)

const swagger2Spec = `{
  "swagger": "2.0",
  "info": {
    "title": "Test API",
    "version": "1.0"
  },
  "basePath": "/api/v1",
  "paths": {
    "/issues": {
      "get": {
        "summary": "List issues",
        "security": [
          {"ApiKeyAuth": []}
        ],
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    },
    "/public": {
      "get": {
        "summary": "Public endpoint",
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    }
  },
  "securityDefinitions": {
    "ApiKeyAuth": {
      "type": "apiKey",
      "name": "Authorization",
      "in": "header"
    }
  }
}`

const openapi3Spec = `{
  "openapi": "3.0.0",
  "info": {
    "title": "Test API",
    "version": "1.0"
  },
  "paths": {
    "/issues": {
      "get": {
        "summary": "List issues",
        "security": [
          {"BearerAuth": []}
        ],
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer"
      },
      "ApiKeyAuth": {
        "type": "apiKey",
        "name": "X-API-Key",
        "in": "header"
      }
    }
  }
}`

func TestGetSecuritySchemes_Swagger2(t *testing.T) {
	doc, err := Parse([]byte(swagger2Spec))
	if err != nil {
		t.Fatalf("Failed to parse swagger 2.0 spec: %v", err)
	}

	// Debug: check what's in the spec
	fmt.Printf("Debug - Components: %+v\n", doc.spec.Components)
	if doc.spec.Components != nil {
		fmt.Printf("Debug - SecuritySchemes: %+v\n", doc.spec.Components.SecuritySchemes)
	}
	fmt.Printf("Debug - Extensions: %+v\n", doc.spec.Extensions)

	schemes := doc.GetSecuritySchemes()
	fmt.Printf("Swagger 2.0 - Found %d security schemes\n", len(schemes))
	for _, scheme := range schemes {
		fmt.Printf("  - Name: %s, Type: %s, Scheme: %s, In: %s, Header: %s\n",
			scheme.Name, scheme.Type, scheme.Scheme, scheme.In, scheme.Header)
	}

	if len(schemes) == 0 {
		t.Error("Expected security schemes to be extracted from Swagger 2.0 spec, got none")
	}
}

func TestGetSecuritySchemes_OpenAPI3(t *testing.T) {
	doc, err := Parse([]byte(openapi3Spec))
	if err != nil {
		t.Fatalf("Failed to parse OpenAPI 3.0 spec: %v", err)
	}

	schemes := doc.GetSecuritySchemes()
	fmt.Printf("OpenAPI 3.0 - Found %d security schemes\n", len(schemes))
	for _, scheme := range schemes {
		fmt.Printf("  - Name: %s, Type: %s, Scheme: %s, In: %s, Header: %s\n",
			scheme.Name, scheme.Type, scheme.Scheme, scheme.In, scheme.Header)
	}

	if len(schemes) != 2 {
		t.Errorf("Expected 2 security schemes from OpenAPI 3.0 spec, got %d", len(schemes))
	}
}

func TestGenerateRequests_WithAuth(t *testing.T) {
	doc, err := Parse([]byte(swagger2Spec))
	if err != nil {
		t.Fatalf("Failed to parse swagger 2.0 spec: %v", err)
	}

	config := GenerationConfig{
		BaseURL:        "http://localhost",
		FuzzingEnabled: false,
	}

	endpoints, err := GenerateRequests(doc, config)
	if err != nil {
		t.Fatalf("Failed to generate requests: %v", err)
	}

	for _, ep := range endpoints {
		fmt.Printf("\n%s %s:\n", ep.Method, ep.Path)
		for _, req := range ep.Requests {
			fmt.Printf("  - %s: %s\n", req.Label, req.URL)
			if len(req.Headers) > 0 {
				fmt.Printf("    Headers:\n")
				for k, v := range req.Headers {
					fmt.Printf("      %s: %s\n", k, v)
				}
			} else {
				fmt.Printf("    Headers: (none)\n")
			}
		}
	}

	// Check that /issues has Authorization header
	for _, ep := range endpoints {
		if ep.Path == "/issues" {
			for _, req := range ep.Requests {
				if _, ok := req.Headers["Authorization"]; !ok {
					t.Error("Expected Authorization header on /issues endpoint")
				}
			}
		}
	}
}

// TestAllAuthTypes tests that all OpenAPI authentication types are properly handled
func TestAllAuthTypes(t *testing.T) {
	allAuthSpec := `{
  "openapi": "3.0.0",
  "info": {"title": "Test API", "version": "1.0"},
  "paths": {
    "/bearer": {
      "get": {
        "summary": "Bearer auth endpoint",
        "security": [{"BearerAuth": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/basic": {
      "get": {
        "summary": "Basic auth endpoint",
        "security": [{"BasicAuth": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/apikey-header": {
      "get": {
        "summary": "API Key in header endpoint",
        "security": [{"ApiKeyHeader": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/apikey-query": {
      "get": {
        "summary": "API Key in query endpoint",
        "security": [{"ApiKeyQuery": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/apikey-cookie": {
      "get": {
        "summary": "API Key in cookie endpoint",
        "security": [{"ApiKeyCookie": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/oauth2": {
      "get": {
        "summary": "OAuth2 endpoint",
        "security": [{"OAuth2": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/openid": {
      "get": {
        "summary": "OpenID Connect endpoint",
        "security": [{"OpenIDConnect": []}],
        "responses": {"200": {"description": "OK"}}
      }
    },
    "/digest": {
      "get": {
        "summary": "Digest auth endpoint",
        "security": [{"DigestAuth": []}],
        "responses": {"200": {"description": "OK"}}
      }
    }
  },
  "components": {
    "securitySchemes": {
      "BearerAuth": {"type": "http", "scheme": "bearer"},
      "BasicAuth": {"type": "http", "scheme": "basic"},
      "DigestAuth": {"type": "http", "scheme": "digest"},
      "ApiKeyHeader": {"type": "apiKey", "name": "X-API-Key", "in": "header"},
      "ApiKeyQuery": {"type": "apiKey", "name": "api_key", "in": "query"},
      "ApiKeyCookie": {"type": "apiKey", "name": "session_token", "in": "cookie"},
      "OAuth2": {
        "type": "oauth2",
        "flows": {
          "authorizationCode": {
            "authorizationUrl": "https://example.com/oauth/authorize",
            "tokenUrl": "https://example.com/oauth/token",
            "scopes": {}
          }
        }
      },
      "OpenIDConnect": {"type": "openIdConnect", "openIdConnectUrl": "https://example.com/.well-known/openid"}
    }
  }
}`

	doc, err := Parse([]byte(allAuthSpec))
	if err != nil {
		t.Fatalf("Failed to parse all-auth spec: %v", err)
	}

	config := GenerationConfig{
		BaseURL:        "http://localhost",
		FuzzingEnabled: false,
	}

	endpoints, err := GenerateRequests(doc, config)
	if err != nil {
		t.Fatalf("Failed to generate requests: %v", err)
	}

	// Define expected results
	expectations := map[string]struct {
		headerKey   string
		headerValue string
		queryKey    string
		queryValue  string
	}{
		"/bearer":        {headerKey: "Authorization", headerValue: "Bearer <TOKEN>"},
		"/basic":         {headerKey: "Authorization", headerValue: "Basic <BASE64_CREDENTIALS>"},
		"/digest":        {headerKey: "Authorization", headerValue: "Digest <DIGEST_CREDENTIALS>"},
		"/apikey-header": {headerKey: "X-API-Key", headerValue: "<API_KEY>"},
		"/apikey-query":  {queryKey: "api_key", queryValue: "<API_KEY>"},
		"/apikey-cookie": {headerKey: "Cookie", headerValue: "session_token=<API_KEY>"},
		"/oauth2":        {headerKey: "Authorization", headerValue: "Bearer <ACCESS_TOKEN>"},
		"/openid":        {headerKey: "Authorization", headerValue: "Bearer <ACCESS_TOKEN>"},
	}

	for _, ep := range endpoints {
		fmt.Printf("\n%s %s:\n", ep.Method, ep.Path)
		for _, req := range ep.Requests {
			fmt.Printf("  URL: %s\n", req.URL)
			if len(req.Headers) > 0 {
				for k, v := range req.Headers {
					fmt.Printf("  Header: %s: %s\n", k, v)
				}
			}
		}

		exp, ok := expectations[ep.Path]
		if !ok {
			continue
		}

		for _, req := range ep.Requests {
			// Check header expectations
			if exp.headerKey != "" {
				if val, ok := req.Headers[exp.headerKey]; !ok {
					t.Errorf("%s: Expected header %s, not found", ep.Path, exp.headerKey)
				} else if val != exp.headerValue {
					t.Errorf("%s: Expected header %s=%s, got %s", ep.Path, exp.headerKey, exp.headerValue, val)
				}
			}

			// Check query param expectations
			if exp.queryKey != "" {
				if !strings.Contains(req.URL, exp.queryKey+"=") {
					t.Errorf("%s: Expected query param %s in URL %s", ep.Path, exp.queryKey, req.URL)
				}
			}
		}
	}
}

func TestGenerateRequests_Deduplication(t *testing.T) {
	spec := `{
  "openapi": "3.0.0",
  "info": {"title": "Test API", "version": "1.0"},
  "paths": {
    "/dedup": {
      "get": {
        "summary": "Deduplication test",
        "parameters": [
          {
            "name": "id",
            "in": "query",
            "schema": {"type": "string", "default": "default"}
          }
        ],
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

	doc, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Failed to parse spec: %v", err)
	}

	config := GenerationConfig{
		BaseURL:        "http://localhost",
		FuzzingEnabled: true, // Enable fuzzing to generate duplicates
	}

	endpoints, err := GenerateRequests(doc, config)
	if err != nil {
		t.Fatalf("Failed to generate requests: %v", err)
	}

	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	ep := endpoints[0]
	// Happy Path = 1
	// Fuzzing "id":
	//   - Baseline (default) -> Duplicate of Happy Path
	//   - Empty
	//   - Simple
	//   - Long
	//   - null
	//   - undefined
	// Total unique should be 1 (Happy) + 5 (Fuzz variations) = 6
	// If deduplication fails, we'd have 7 (including the baseline duplicate)

	// Let's just check that we don't have duplicates by URL
	seenURLs := make(map[string]bool)
	for _, req := range ep.Requests {
		if seenURLs[req.URL] {
			t.Errorf("Found duplicate URL: %s (Label: %s)", req.URL, req.Label)
		}
		seenURLs[req.URL] = true
	}
}

func TestGenerateRequests_CookieParams(t *testing.T) {
	spec := `{
  "openapi": "3.0.0",
  "info": {"title": "Test API", "version": "1.0"},
  "paths": {
    "/cookies": {
      "get": {
        "summary": "Cookie test",
        "parameters": [
          {
            "name": "session_id",
            "in": "cookie",
            "schema": {"type": "string", "default": "sess_123"}
          },
          {
            "name": "theme",
            "in": "cookie",
            "schema": {"type": "string", "default": "dark"}
          }
        ],
        "security": [{"ApiKeyCookie": []}],
        "responses": {"200": {"description": "OK"}}
      }
    }
  },
  "components": {
    "securitySchemes": {
      "ApiKeyCookie": {"type": "apiKey", "name": "auth_token", "in": "cookie"}
    }
  }
}`

	doc, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Failed to parse spec: %v", err)
	}

	config := GenerationConfig{
		BaseURL:        "http://localhost",
		FuzzingEnabled: false,
	}

	endpoints, err := GenerateRequests(doc, config)
	if err != nil {
		t.Fatalf("Failed to generate requests: %v", err)
	}

	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	req := endpoints[0].Requests[0]
	cookieHeader := req.Headers["Cookie"]

	// Expected cookies: auth_token=<API_KEY>, session_id=sess_123, theme=dark
	// Order might vary, so check for presence
	if !strings.Contains(cookieHeader, "auth_token=<API_KEY>") {
		t.Errorf("Cookie header missing auth_token: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "session_id=sess_123") {
		t.Errorf("Cookie header missing session_id: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "theme=dark") {
		t.Errorf("Cookie header missing theme: %s", cookieHeader)
	}

	// Check delimiter
	if strings.Count(cookieHeader, "; ") != 2 {
		t.Errorf("Cookie header should have 2 delimiters '; ', got: %s", cookieHeader)
	}
}
