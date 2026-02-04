package graphql

import (
	"encoding/json"
	"strings"
	"testing"

	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
)

func TestAnalyzeResponse(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		want       bool
	}{
		{
			name:       "successful response with data",
			body:       `{"data":{"users":[{"id":"1","name":"Alice"}]}}`,
			statusCode: 200,
			want:       true,
		},
		{
			name:       "null data field",
			body:       `{"data":null}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "data with all null values",
			body:       `{"data":{"users":null}}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "400 status code",
			body:       `{"errors":[{"message":"bad request"}]}`,
			statusCode: 400,
			want:       false,
		},
		{
			name:       "500 status code",
			body:       `{"data":{"users":[{"id":"1"}]}}`,
			statusCode: 500,
			want:       false,
		},
		{
			name:       "depth rejection error",
			body:       `{"errors":[{"message":"Query exceeds maximum depth of 10"}],"data":null}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "depth limit error",
			body:       `{"errors":[{"message":"depth limit exceeded"}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "syntax error",
			body:       `{"errors":[{"message":"Syntax Error: Unexpected token"}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "unknown field validation error",
			body:       `{"errors":[{"message":"Cannot query field \"foo\" on type \"Query\""}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "invalid JSON body",
			body:       `not json at all`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "empty body",
			body:       ``,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "no data field at all",
			body:       `{"errors":[{"message":"some random error"}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "data present with non-depth errors",
			body:       `{"errors":[{"message":"some warning"}],"data":{"users":[{"id":"1"}]}}`,
			statusCode: 200,
			want:       true,
		},
		{
			name:       "extensions code depth rejection",
			body:       `{"errors":[{"message":"query rejected","extensions":{"code":"DEPTH_LIMIT_EXCEEDED"}}],"data":null}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "extensions code complexity rejection",
			body:       `{"errors":[{"message":"too complex","extensions":{"code":"QUERY_COMPLEXITY"}}],"data":null}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "data is not a map",
			body:       `{"data":"string_value"}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "data with non-null nested value",
			body:       `{"data":{"__schema":{"types":[{"name":"Query"}]}}}`,
			statusCode: 200,
			want:       true,
		},
		{
			name:       "too deep error message",
			body:       `{"errors":[{"message":"Query is too deep"}],"data":null}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "nesting too deep",
			body:       `{"errors":[{"message":"Nesting too deep, maximum allowed is 5"}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "parse error in response",
			body:       `{"errors":[{"message":"Parse error on line 1"}]}`,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "validation error - is not defined",
			body:       `{"errors":[{"message":"Field 'foo' is not defined on type 'Query'"}]}`,
			statusCode: 200,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeResponse([]byte(tt.body), tt.statusCode)
			if got != tt.want {
				t.Errorf("analyzeResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDepthRejection(t *testing.T) {
	tests := []struct {
		name   string
		errors []map[string]any
		want   bool
	}{
		{
			name:   "maximum query depth message",
			errors: []map[string]any{{"message": "Maximum query depth of 10 exceeded"}},
			want:   true,
		},
		{
			name:   "depth limit message",
			errors: []map[string]any{{"message": "Depth limit of 5 reached"}},
			want:   true,
		},
		{
			name:   "too deep message",
			errors: []map[string]any{{"message": "Query is too deep"}},
			want:   true,
		},
		{
			name:   "query too complex message",
			errors: []map[string]any{{"message": "Query too complex to process"}},
			want:   true,
		},
		{
			name:   "exceeds maximum depth",
			errors: []map[string]any{{"message": "Query exceeds maximum depth"}},
			want:   true,
		},
		{
			name:   "nesting too deep",
			errors: []map[string]any{{"message": "Nesting too deep for this endpoint"}},
			want:   true,
		},
		{
			name:   "max depth message",
			errors: []map[string]any{{"message": "Reached max depth"}},
			want:   true,
		},
		{
			name:   "unrelated error message",
			errors: []map[string]any{{"message": "User not found"}},
			want:   false,
		},
		{
			name:   "empty error message",
			errors: []map[string]any{{"message": ""}},
			want:   false,
		},
		{
			name:   "no message field",
			errors: []map[string]any{{"code": "NOT_FOUND"}},
			want:   false,
		},
		{
			name: "depth in extensions code",
			errors: []map[string]any{{
				"message":    "rejected",
				"extensions": map[string]any{"code": "DEPTH_LIMIT"},
			}},
			want: true,
		},
		{
			name: "complexity in extensions code",
			errors: []map[string]any{{
				"message":    "rejected",
				"extensions": map[string]any{"code": "COMPLEXITY_EXCEEDED"},
			}},
			want: true,
		},
		{
			name:   "word depth alone is not matched",
			errors: []map[string]any{{"message": "in-depth analysis required"}},
			want:   false,
		},
		{
			name:   "word limit alone is not matched",
			errors: []map[string]any{{"message": "rate limit exceeded"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDepthRejection(tt.errors)
			if got != tt.want {
				t.Errorf("isDepthRejection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsSyntaxOrValidationError(t *testing.T) {
	tests := []struct {
		name   string
		errors []map[string]any
		want   bool
	}{
		{
			name:   "syntax error",
			errors: []map[string]any{{"message": "Syntax Error: Unexpected '}'"}},
			want:   true,
		},
		{
			name:   "parse error",
			errors: []map[string]any{{"message": "Parse error at position 42"}},
			want:   true,
		},
		{
			name:   "unexpected token",
			errors: []map[string]any{{"message": "Unexpected end of document"}},
			want:   true,
		},
		{
			name:   "cannot query field",
			errors: []map[string]any{{"message": "Cannot query field 'foo' on type 'Bar'"}},
			want:   true,
		},
		{
			name:   "unknown field",
			errors: []map[string]any{{"message": "Unknown field 'baz' on type 'Query'"}},
			want:   true,
		},
		{
			name:   "validation error",
			errors: []map[string]any{{"message": "Validation error: field required"}},
			want:   true,
		},
		{
			name:   "is not defined",
			errors: []map[string]any{{"message": "Type 'Foo' is not defined in the schema"}},
			want:   true,
		},
		{
			name:   "did you mean suggestion",
			errors: []map[string]any{{"message": "Did you mean 'users'?"}},
			want:   true,
		},
		{
			name:   "normal business error",
			errors: []map[string]any{{"message": "User not found"}},
			want:   false,
		},
		{
			name:   "permission error",
			errors: []map[string]any{{"message": "Not authorized to access this resource"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsSyntaxOrValidationError(tt.errors)
			if got != tt.want {
				t.Errorf("containsSyntaxOrValidationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindDeepTypeChains(t *testing.T) {
	t.Run("cyclic types detected", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "user",
					ReturnType: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject},
				},
			},
			Types: map[string]pkgGraphql.TypeDef{
				"User": {
					Name: "User",
					Fields: []pkgGraphql.Field{
						{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "posts", Type: pkgGraphql.TypeRef{Name: "Post", Kind: pkgGraphql.TypeKindObject}},
					},
				},
				"Post": {
					Name: "Post",
					Fields: []pkgGraphql.Field{
						{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "title", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "author", Type: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject}},
					},
				},
			},
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		chains := findDeepTypeChains(schema)
		if len(chains) == 0 {
			t.Fatal("Expected at least one chain, got none")
		}

		hasCyclic := false
		for _, c := range chains {
			if c.cyclic {
				hasCyclic = true
				break
			}
		}
		if !hasCyclic {
			t.Error("Expected at least one cyclic chain")
		}
	})

	t.Run("no object types returns empty", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "version",
					ReturnType: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar},
				},
			},
			Types: make(map[string]pkgGraphql.TypeDef),
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		chains := findDeepTypeChains(schema)
		if len(chains) != 0 {
			t.Errorf("Expected 0 chains, got %d", len(chains))
		}
	})

	t.Run("deep non-cyclic chain", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "company",
					ReturnType: pkgGraphql.TypeRef{Name: "Company", Kind: pkgGraphql.TypeKindObject},
				},
			},
			Types: map[string]pkgGraphql.TypeDef{
				"Company": {
					Name: "Company",
					Fields: []pkgGraphql.Field{
						{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "department", Type: pkgGraphql.TypeRef{Name: "Department", Kind: pkgGraphql.TypeKindObject}},
					},
				},
				"Department": {
					Name: "Department",
					Fields: []pkgGraphql.Field{
						{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "team", Type: pkgGraphql.TypeRef{Name: "Team", Kind: pkgGraphql.TypeKindObject}},
					},
				},
				"Team": {
					Name: "Team",
					Fields: []pkgGraphql.Field{
						{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
					},
				},
			},
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		chains := findDeepTypeChains(schema)
		if len(chains) != 0 {
			for _, c := range chains {
				if c.cyclic {
					t.Error("Did not expect cyclic chain for non-cyclic schema")
				}
			}
		}
	})

	t.Run("cyclic chains sorted first", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "user",
					ReturnType: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject},
				},
			},
			Types: map[string]pkgGraphql.TypeDef{
				"User": {
					Name: "User",
					Fields: []pkgGraphql.Field{
						{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "friends", Type: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject}},
					},
				},
			},
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		chains := findDeepTypeChains(schema)
		if len(chains) > 0 && !chains[0].cyclic {
			t.Error("Expected cyclic chains to be sorted first")
		}
	})
}

func TestGetSchemaAwareDepthTestCases(t *testing.T) {
	t.Run("includes generic tests plus schema-aware", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "user",
					ReturnType: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject},
				},
			},
			Types: map[string]pkgGraphql.TypeDef{
				"User": {
					Name: "User",
					Fields: []pkgGraphql.Field{
						{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
						{Name: "friends", Type: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject}},
					},
				},
			},
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		testCases := getSchemaAwareDepthTestCases(schema)

		genericCount := len(getGenericDepthTestCases())
		if len(testCases) <= genericCount {
			t.Errorf("Expected more test cases than generic (%d), got %d", genericCount, len(testCases))
		}

		hasSchemaTest := false
		for _, tc := range testCases {
			if len(tc.name) > 7 && tc.name[:7] == "schema_" {
				hasSchemaTest = true
				break
			}
		}
		if !hasSchemaTest {
			t.Error("Expected at least one schema-aware test case")
		}
	})

	t.Run("falls back to generic when no chains", func(t *testing.T) {
		schema := &pkgGraphql.GraphQLSchema{
			Queries: []pkgGraphql.Operation{
				{
					Name:       "version",
					ReturnType: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar},
				},
			},
			Types: make(map[string]pkgGraphql.TypeDef),
			Enums: make(map[string]pkgGraphql.EnumDef),
		}

		testCases := getSchemaAwareDepthTestCases(schema)
		genericCount := len(getGenericDepthTestCases())
		if len(testCases) != genericCount {
			t.Errorf("Expected exactly generic test count (%d), got %d", genericCount, len(testCases))
		}
	})
}

func TestBuildDeepQueryFromChain(t *testing.T) {
	schema := &pkgGraphql.GraphQLSchema{
		Types: map[string]pkgGraphql.TypeDef{
			"User": {
				Name: "User",
				Fields: []pkgGraphql.Field{
					{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "friends", Type: pkgGraphql.TypeRef{Name: "User", Kind: pkgGraphql.TypeKindObject}},
				},
			},
		},
		Scalars: []string{},
		Enums:   make(map[string]pkgGraphql.EnumDef),
	}

	t.Run("cyclic chain generates valid query", func(t *testing.T) {
		chain := typeChain{
			rootField: "user",
			steps: []chainStep{
				{fieldName: "friends", typeName: "User"},
			},
			cyclic: true,
		}

		query := buildDeepQueryFromChain(schema, chain, 5)
		if query == "" {
			t.Fatal("Expected non-empty query")
		}

		if !containsBalancedBraces(query) {
			t.Errorf("Query has unbalanced braces: %s", query)
		}
	})

	t.Run("empty steps returns empty", func(t *testing.T) {
		chain := typeChain{
			rootField: "user",
			steps:     nil,
			cyclic:    true,
		}

		query := buildDeepQueryFromChain(schema, chain, 5)
		if query != "" {
			t.Errorf("Expected empty query for empty steps, got: %s", query)
		}
	})
}

func TestFindScalarField(t *testing.T) {
	schema := &pkgGraphql.GraphQLSchema{
		Types: map[string]pkgGraphql.TypeDef{
			"User": {
				Name: "User",
				Fields: []pkgGraphql.Field{
					{Name: "age", Type: pkgGraphql.TypeRef{Name: "Int", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "posts", Type: pkgGraphql.TypeRef{Name: "Post", Kind: pkgGraphql.TypeKindObject}},
				},
			},
			"Post": {
				Name: "Post",
				Fields: []pkgGraphql.Field{
					{Name: "content", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
				},
			},
		},
		Scalars: []string{},
		Enums:   make(map[string]pkgGraphql.EnumDef),
	}

	t.Run("prefers id field", func(t *testing.T) {
		field := findScalarField(schema, "User")
		if field != "id" {
			t.Errorf("Expected 'id', got '%s'", field)
		}
	})

	t.Run("falls back to any scalar", func(t *testing.T) {
		field := findScalarField(schema, "Post")
		if field != "content" {
			t.Errorf("Expected 'content', got '%s'", field)
		}
	})

	t.Run("unknown type returns id", func(t *testing.T) {
		field := findScalarField(schema, "Unknown")
		if field != "id" {
			t.Errorf("Expected 'id' for unknown type, got '%s'", field)
		}
	})
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name    string
		results []depthTestResult
		wantMin int
		wantMax int
	}{
		{
			name: "depth 8 baseline",
			results: []depthTestResult{
				{testName: "introspection_depth_8", depth: 8, passed: true},
			},
			wantMin: 75,
			wantMax: 75,
		},
		{
			name: "depth 12 test",
			results: []depthTestResult{
				{testName: "introspection_depth_12", depth: 12, passed: true},
			},
			wantMin: 80,
			wantMax: 80,
		},
		{
			name: "depth 20 high confidence",
			results: []depthTestResult{
				{testName: "introspection_depth_20", depth: 20, passed: true},
			},
			wantMin: 85,
			wantMax: 85,
		},
		{
			name: "schema-aware adds confidence",
			results: []depthTestResult{
				{testName: "schema_user_depth_10", depth: 10, passed: true},
			},
			wantMin: 85,
			wantMax: 85,
		},
		{
			name: "multiple diverse tests",
			results: []depthTestResult{
				{testName: "introspection_depth_8", depth: 8, passed: true},
				{testName: "schema_user_depth_10", depth: 10, passed: true},
				{testName: "schema_user_depth_13", depth: 13, passed: true},
			},
			wantMin: 95,
			wantMax: 95,
		},
		{
			name: "capped at 95",
			results: []depthTestResult{
				{testName: "schema_user_depth_20", depth: 20, passed: true},
				{testName: "schema_post_depth_20", depth: 20, passed: true},
				{testName: "self_reference", depth: 999, passed: true},
				{testName: "introspection_depth_20", depth: 20, passed: true},
			},
			wantMin: 95,
			wantMax: 95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateConfidence(tt.results)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calculateConfidence() = %d, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGetGenericDepthTestCases(t *testing.T) {
	cases := getGenericDepthTestCases()
	if len(cases) != 5 {
		t.Errorf("Expected 5 generic test cases, got %d", len(cases))
	}

	for _, tc := range cases {
		if tc.name == "" {
			t.Error("Test case has empty name")
		}
		if tc.query == "" {
			t.Error("Test case has empty query")
		}
		if tc.depth <= 0 {
			t.Errorf("Test case %s has invalid depth %d", tc.name, tc.depth)
		}

		if !isValidJSON(tc.query) {
			t.Errorf("Test case %s has invalid JSON query: %s", tc.name, tc.query)
		}
	}
}

func TestGetCircularFragmentTestCases(t *testing.T) {
	cases := getCircularFragmentTestCases()
	if len(cases) != 3 {
		t.Errorf("Expected 3 circular fragment test cases, got %d", len(cases))
	}

	for _, tc := range cases {
		if tc.depth != 999 {
			t.Errorf("Circular test %s should have depth 999, got %d", tc.name, tc.depth)
		}
		if !isValidJSON(tc.query) {
			t.Errorf("Test case %s has invalid JSON query: %s", tc.name, tc.query)
		}
	}
}

func TestBuildConsolidatedDetails(t *testing.T) {
	results := []depthTestResult{
		{testName: "introspection_depth_8", depth: 8, description: "8-level nested"},
		{testName: "self_reference", depth: 999, description: "Direct self-referencing fragment"},
	}

	details := buildConsolidatedDetails(results)

	expectedSubstrings := []string{
		"depth limit is not enforced",
		"999",
		"introspection_depth_8",
		"self_reference",
		"infinite (circular)",
	}

	for _, substr := range expectedSubstrings {
		if !containsStr(details, substr) {
			t.Errorf("Expected details to contain '%s'", substr)
		}
	}
}

func isValidJSON(s string) bool {
	var js map[string]any
	return json.Unmarshal([]byte(s), &js) == nil
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}

func containsBalancedBraces(s string) bool {
	count := 0
	for _, ch := range s {
		if ch == '{' {
			count++
		} else if ch == '}' {
			count--
		}
		if count < 0 {
			return false
		}
	}
	return count == 0
}
