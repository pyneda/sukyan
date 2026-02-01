package api

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestDetectAPIType(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		sourceURL string
		want      db.APIDefinitionType
	}{
		{
			name:      "openapi json with openapi key",
			content:   `{"openapi": "3.0.0", "info": {"title": "Test API"}}`,
			sourceURL: "https://example.com/api.json",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "openapi json with swagger key",
			content:   `{"swagger": "2.0", "info": {"title": "Test API"}}`,
			sourceURL: "https://example.com/swagger.json",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "openapi yaml with openapi key",
			content:   "openapi: '3.1.0'\ninfo:\n  title: Test API\n",
			sourceURL: "https://example.com/api.yaml",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "openapi yaml with swagger key",
			content:   "swagger: '2.0'\ninfo:\n  title: Test API\n",
			sourceURL: "https://example.com/api.yaml",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "graphql introspection with data.__schema",
			content:   `{"data": {"__schema": {"queryType": {"name": "Query"}}}}`,
			sourceURL: "https://example.com/api",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "graphql introspection with top-level __schema",
			content:   `{"__schema": {"queryType": {"name": "Query"}, "types": []}}`,
			sourceURL: "https://example.com/api",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "graphql with queryType in content",
			content:   `{"types": [{"name": "Query"}], "queryType": {"name": "Query"}}`,
			sourceURL: "https://example.com/api",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "graphql with mutationType in content",
			content:   `some schema with mutationType defined`,
			sourceURL: "https://example.com/api",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "graphql detected from url hint",
			content:   `{"some": "unknown content"}`,
			sourceURL: "https://example.com/graphql",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "graphql detected from url with path",
			content:   `{"some": "data"}`,
			sourceURL: "https://example.com/api/graphql/v1",
			want:      db.APIDefinitionTypeGraphQL,
		},
		{
			name:      "wsdl from xml content",
			content:   `<?xml version="1.0"?><wsdl:definitions xmlns:wsdl="http://schemas.xmlsoap.org/wsdl/"></wsdl:definitions>`,
			sourceURL: "https://example.com/service",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "wsdl from soap content",
			content:   `<?xml version="1.0"?><definitions><soap:binding style="document"/></definitions>`,
			sourceURL: "https://example.com/service",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "wsdl from definitions tag",
			content:   `<?xml version="1.0"?><definitions name="MyService" targetNamespace="http://example.com"></definitions>`,
			sourceURL: "https://example.com/service",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "wsdl from url ending in .wsdl",
			content:   `{"some": "data"}`,
			sourceURL: "https://example.com/service.wsdl",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "wsdl from url with ?wsdl query",
			content:   `{"some": "data"}`,
			sourceURL: "https://example.com/service?wsdl",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "wsdl url detection is case insensitive",
			content:   `{"some": "data"}`,
			sourceURL: "https://example.com/Service.WSDL",
			want:      db.APIDefinitionTypeWSDL,
		},
		{
			name:      "defaults to openapi for unrecognized content",
			content:   `{"endpoints": ["/users", "/posts"]}`,
			sourceURL: "https://example.com/api-spec.json",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "defaults to openapi for empty content",
			content:   "",
			sourceURL: "https://example.com/spec",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "openapi keyword takes priority over graphql url",
			content:   `{"openapi": "3.0.0", "paths": {}}`,
			sourceURL: "https://example.com/graphql/openapi.json",
			want:      db.APIDefinitionTypeOpenAPI,
		},
		{
			name:      "wsdl content takes priority over graphql url",
			content:   `<wsdl:definitions></wsdl:definitions>`,
			sourceURL: "https://example.com/graphql",
			want:      db.APIDefinitionTypeWSDL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAPIType([]byte(tt.content), tt.sourceURL)
			if got != tt.want {
				t.Errorf("DetectAPIType() = %q, want %q", got, tt.want)
			}
		})
	}
}
