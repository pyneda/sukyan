package scan

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/lib"
)

func TestIsGraphQLBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "query operation",
			body:     `{"query": "query GetUser($id: ID!) { user(id: $id) { name } }", "variables": {"id": "1"}}`,
			expected: true,
		},
		{
			name:     "mutation operation",
			body:     `{"query": "mutation CreateUser($name: String!) { createUser(name: $name) { id } }", "variables": {"name": "test"}}`,
			expected: true,
		},
		{
			name:     "subscription operation",
			body:     `{"query": "subscription OnMessage { messageAdded { text } }"}`,
			expected: true,
		},
		{
			name:     "shorthand query",
			body:     `{"query": "{ users { id name } }"}`,
			expected: true,
		},
		{
			name:     "query with leading whitespace",
			body:     `{"query": "  query GetUser { user { name } }"}`,
			expected: true,
		},
		{
			name:     "regular JSON body",
			body:     `{"name": "test", "email": "test@example.com"}`,
			expected: false,
		},
		{
			name:     "JSON with query key but not graphql",
			body:     `{"query": "SELECT * FROM users"}`,
			expected: false,
		},
		{
			name:     "query key with non-string value",
			body:     `{"query": 123}`,
			expected: false,
		},
		{
			name:     "no query key",
			body:     `{"variables": {"id": "1"}}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jsonData map[string]any
			if err := json.Unmarshal([]byte(tt.body), &jsonData); err != nil {
				t.Fatalf("Failed to parse test body: %v", err)
			}
			result := isGraphQLBody(jsonData)
			if result != tt.expected {
				t.Errorf("isGraphQLBody() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractGraphQLVariablePoints(t *testing.T) {
	tests := []struct {
		name           string
		variables      map[string]any
		expectedCount  int
		expectedNames  []string
		expectedValues []string
	}{
		{
			name:           "flat string variables",
			variables:      map[string]any{"id": "123", "name": "test"},
			expectedCount:  2,
			expectedNames:  []string{"id", "name"},
			expectedValues: []string{"123", "test"},
		},
		{
			name:           "mixed types",
			variables:      map[string]any{"id": "abc", "count": float64(42), "active": true},
			expectedCount:  3,
			expectedNames:  []string{"id", "count", "active"},
			expectedValues: []string{"abc", "42", "true"},
		},
		{
			name: "nested object",
			variables: map[string]any{
				"id": "1",
				"filter": map[string]any{
					"status": "active",
					"limit":  float64(10),
				},
			},
			expectedCount:  3,
			expectedNames:  []string{"id", "filter.status", "filter.limit"},
			expectedValues: []string{"1", "active", "10"},
		},
		{
			name: "deeply nested",
			variables: map[string]any{
				"input": map[string]any{
					"user": map[string]any{
						"name": "test",
					},
				},
			},
			expectedCount:  1,
			expectedNames:  []string{"input.user.name"},
			expectedValues: []string{"test"},
		},
		{
			name: "array variable",
			variables: map[string]any{
				"ids": []any{"a", "b", "c"},
			},
			expectedCount:  3,
			expectedNames:  []string{"ids[0]", "ids[1]", "ids[2]"},
			expectedValues: []string{"a", "b", "c"},
		},
		{
			name: "array of objects",
			variables: map[string]any{
				"items": []any{
					map[string]any{"name": "first"},
					map[string]any{"name": "second"},
				},
			},
			expectedCount:  2,
			expectedNames:  []string{"items[0].name", "items[1].name"},
			expectedValues: []string{"first", "second"},
		},
		{
			name:           "empty variables",
			variables:      map[string]any{},
			expectedCount:  0,
			expectedNames:  nil,
			expectedValues: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := extractGraphQLVariablePoints("", tt.variables, `{"query":"test","variables":{}}`)

			if len(points) != tt.expectedCount {
				t.Errorf("Expected %d insertion points, got %d", tt.expectedCount, len(points))
				for _, p := range points {
					t.Logf("  Point: %s = %s", p.Name, p.Value)
				}
				return
			}

			for _, p := range points {
				if p.Type != InsertionPointTypeGraphQLVariable {
					t.Errorf("Expected type %s, got %s for point %s", InsertionPointTypeGraphQLVariable, p.Type, p.Name)
				}
			}

			pointMap := make(map[string]string)
			for _, p := range points {
				pointMap[p.Name] = p.Value
			}

			for i, name := range tt.expectedNames {
				val, ok := pointMap[name]
				if !ok {
					t.Errorf("Expected insertion point %q not found", name)
					continue
				}
				if val != tt.expectedValues[i] {
					t.Errorf("Point %q: expected value %q, got %q", name, tt.expectedValues[i], val)
				}
			}
		})
	}
}

func TestExtractGraphQLVariablePointsValueTypes(t *testing.T) {
	variables := map[string]any{
		"strVal":  "hello",
		"numVal":  float64(42),
		"boolVal": true,
	}

	points := extractGraphQLVariablePoints("", variables, "{}")
	pointMap := make(map[string]InsertionPoint)
	for _, p := range points {
		pointMap[p.Name] = p
	}

	if p, ok := pointMap["strVal"]; ok {
		if p.ValueType != lib.TypeString {
			t.Errorf("strVal: expected type %s, got %s", lib.TypeString, p.ValueType)
		}
	}

	if p, ok := pointMap["numVal"]; ok {
		if p.ValueType != lib.TypeInt && p.ValueType != lib.TypeFloat {
			t.Errorf("numVal: expected numeric type, got %s", p.ValueType)
		}
	}
}

func TestModifyGraphQLVariables(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		pointName       string
		payload         string
		expectedVarPath string
		expectedValue   any
	}{
		{
			name:            "modify flat variable",
			body:            `{"query":"query GetUser($id: ID!) { user(id: $id) { name } }","variables":{"id":"123"}}`,
			pointName:       "id",
			payload:         "' OR '1'='1",
			expectedVarPath: "id",
			expectedValue:   "' OR '1'='1",
		},
		{
			name:            "modify nested variable",
			body:            `{"query":"mutation Test($input: Input!) { test(input: $input) { ok } }","variables":{"input":{"name":"test","email":"test@example.com"}}}`,
			pointName:       "input.email",
			payload:         "<script>alert(1)</script>",
			expectedVarPath: "input.email",
			expectedValue:   "<script>alert(1)</script>",
		},
		{
			name:            "modify array element",
			body:            `{"query":"query Test($ids: [ID!]!) { items(ids: $ids) { id } }","variables":{"ids":["a","b","c"]}}`,
			pointName:       "ids[1]",
			payload:         "injected",
			expectedVarPath: "ids[1]",
			expectedValue:   "injected",
		},
		{
			name:            "preserves numeric type",
			body:            `{"query":"query Test($limit: Int!) { items(limit: $limit) { id } }","variables":{"limit":10}}`,
			pointName:       "limit",
			payload:         "99999",
			expectedVarPath: "limit",
			expectedValue:   float64(99999),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := InsertionPointBuilder{
				Point: InsertionPoint{
					Type: InsertionPointTypeGraphQLVariable,
					Name: tt.pointName,
				},
				Payload: tt.payload,
			}

			result, err := modifyGraphQLVariables([]byte(tt.body), []InsertionPointBuilder{builder})
			if err != nil {
				t.Fatalf("modifyGraphQLVariables() error: %v", err)
			}

			var parsed map[string]any
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}

			if _, ok := parsed["query"]; !ok {
				t.Error("Result missing 'query' field")
			}

			vars, ok := parsed["variables"].(map[string]any)
			if !ok {
				t.Fatal("Result missing or invalid 'variables' field")
			}

			val := getNestedValue(vars, tt.expectedVarPath)
			if val != tt.expectedValue {
				t.Errorf("Variable %q = %v (%T), want %v (%T)", tt.expectedVarPath, val, val, tt.expectedValue, tt.expectedValue)
			}
		})
	}
}

func TestModifyGraphQLVariablesPreservesStructure(t *testing.T) {
	body := `{"query":"query Test($id: ID!, $name: String!) { user(id: $id, name: $name) { email } }","variables":{"id":"1","name":"original"},"operationName":"Test"}`

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeGraphQLVariable,
			Name: "id",
		},
		Payload: "injected",
	}

	result, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if parsed["operationName"] != "Test" {
		t.Errorf("operationName was modified: got %v", parsed["operationName"])
	}

	vars := parsed["variables"].(map[string]any)
	if vars["name"] != "original" {
		t.Errorf("Unrelated variable 'name' was modified: got %v", vars["name"])
	}
	if vars["id"] != "injected" {
		t.Errorf("Target variable 'id' not modified: got %v", vars["id"])
	}
}

func TestModifyGraphQLVariablesEdgeCases(t *testing.T) {
	t.Run("null variables field", func(t *testing.T) {
		body := `{"query":"query Test { users { id } }","variables":null}`
		builder := InsertionPointBuilder{
			Point:   InsertionPoint{Type: InsertionPointTypeGraphQLVariable, Name: "id"},
			Payload: "injected",
		}
		result, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(result, &parsed); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}
		if _, ok := parsed["query"]; !ok {
			t.Error("query field missing")
		}
	})

	t.Run("missing variables field", func(t *testing.T) {
		body := `{"query":"query Test { users { id } }"}`
		builder := InsertionPointBuilder{
			Point:   InsertionPoint{Type: InsertionPointTypeGraphQLVariable, Name: "id"},
			Payload: "injected",
		}
		result, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(result, &parsed); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}
		vars, ok := parsed["variables"].(map[string]any)
		if !ok {
			t.Fatal("variables field not created")
		}
		if vars["id"] != "injected" {
			t.Errorf("expected 'injected', got %v", vars["id"])
		}
	})

	t.Run("missing query field", func(t *testing.T) {
		body := `{"variables":{"id":"123"}}`
		builder := InsertionPointBuilder{
			Point:   InsertionPoint{Type: InsertionPointTypeGraphQLVariable, Name: "id"},
			Payload: "test",
		}
		_, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
		if err == nil {
			t.Error("expected error for missing query field")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		body := `{"query":"query Test { users }`
		builder := InsertionPointBuilder{
			Point:   InsertionPoint{Type: InsertionPointTypeGraphQLVariable, Name: "id"},
			Payload: "test",
		}
		_, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
		if err == nil {
			t.Error("expected error for malformed JSON")
		}
	})

	t.Run("nil original value coercion", func(t *testing.T) {
		body := `{"query":"query Test($id: ID) { user(id: $id) { name } }","variables":{"id":null}}`
		builder := InsertionPointBuilder{
			Point:   InsertionPoint{Type: InsertionPointTypeGraphQLVariable, Name: "id"},
			Payload: "injected",
		}
		result, err := modifyGraphQLVariables([]byte(body), []InsertionPointBuilder{builder})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(result, &parsed); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}
		vars := parsed["variables"].(map[string]any)
		if vars["id"] != "injected" {
			t.Errorf("expected 'injected', got %v", vars["id"])
		}
	})
}

func getNestedValue(obj map[string]any, path string) any {
	parts := splitDotPath(path)

	for i, part := range parts {
		if idx, name, isArray := parseGraphQLArrayAccess(part); isArray {
			arr, ok := obj[name].([]any)
			if !ok || idx >= len(arr) {
				return nil
			}
			if i == len(parts)-1 {
				return arr[idx]
			}
			nested, ok := arr[idx].(map[string]any)
			if !ok {
				return nil
			}
			obj = nested
			continue
		}

		if i == len(parts)-1 {
			return obj[part]
		}
		nested, ok := obj[part].(map[string]any)
		if !ok {
			return nil
		}
		obj = nested
	}
	return nil
}

func splitDotPath(path string) []string {
	var parts []string
	current := ""
	for _, ch := range path {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func TestExtractGraphQLInlineArgPoints(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedNames  []string
		expectedValues []string
	}{
		{
			name:           "string literal",
			query:          `mutation CreateUser($email: String!) { createUser(email: $email, role: "admin") { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"role"},
			expectedValues: []string{"admin"},
		},
		{
			name:           "number literal",
			query:          `query GetUsers { users(limit: 10) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"limit"},
			expectedValues: []string{"10"},
		},
		{
			name:           "boolean literal",
			query:          `query GetUsers { users(active: true) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"active"},
			expectedValues: []string{"true"},
		},
		{
			name:           "null literal",
			query:          `query GetUser { user(deletedAt: null) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"deletedAt"},
			expectedValues: []string{"null"},
		},
		{
			name:           "enum value",
			query:          `query GetUsers { users(status: ACTIVE) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"status"},
			expectedValues: []string{"ACTIVE"},
		},
		{
			name:           "skip variable references",
			query:          `mutation CreateUser($email: String!) { createUser(email: $email) { id } }`,
			expectedCount:  0,
			expectedNames:  nil,
			expectedValues: nil,
		},
		{
			name:           "mixed variable refs and literals",
			query:          `mutation CreateUser($email: String!) { createUser(email: $email, role: "admin", limit: 10) { id } }`,
			expectedCount:  2,
			expectedNames:  []string{"role", "limit"},
			expectedValues: []string{"admin", "10"},
		},
		{
			name:           "multiple field arguments",
			query:          `query Test { users(limit: 10) { posts(status: "published") { title } } }`,
			expectedCount:  2,
			expectedNames:  []string{"limit", "status"},
			expectedValues: []string{"10", "published"},
		},
		{
			name:           "no arguments",
			query:          `query GetUsers { users { id name } }`,
			expectedCount:  0,
			expectedNames:  nil,
			expectedValues: nil,
		},
		{
			name:           "escaped string value",
			query:          `query Test { search(query: "hello \"world\"") { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"query"},
			expectedValues: []string{`hello "world"`},
		},
		{
			name:           "float literal",
			query:          `query Test { items(minPrice: 9.99) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"minPrice"},
			expectedValues: []string{"9.99"},
		},
		{
			name:           "negative number",
			query:          `query Test { items(offset: -1) { id } }`,
			expectedCount:  1,
			expectedNames:  []string{"offset"},
			expectedValues: []string{"-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := extractGraphQLInlineArgPoints(tt.query, `{"query":"test"}`)

			if len(points) != tt.expectedCount {
				t.Errorf("Expected %d insertion points, got %d", tt.expectedCount, len(points))
				for _, p := range points {
					t.Logf("  Point: %s = %q", p.Name, p.Value)
				}
				return
			}

			for _, p := range points {
				if p.Type != InsertionPointTypeGraphQLInlineArg {
					t.Errorf("Expected type %s, got %s for point %s", InsertionPointTypeGraphQLInlineArg, p.Type, p.Name)
				}
			}

			pointMap := make(map[string]string)
			for _, p := range points {
				pointMap[p.Name] = p.Value
			}

			for i, name := range tt.expectedNames {
				val, ok := pointMap[name]
				if !ok {
					t.Errorf("Expected insertion point %q not found", name)
					continue
				}
				if val != tt.expectedValues[i] {
					t.Errorf("Point %q: expected value %q, got %q", name, tt.expectedValues[i], val)
				}
			}
		})
	}
}

func TestModifyGraphQLInlineArg(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		argName       string
		payload       string
		expectInQuery string
	}{
		{
			name:          "replace string literal",
			body:          `{"query":"mutation CreateUser($email: String!) { createUser(email: $email, role: \"admin\") { id } }","variables":{"email":"test@test.com"}}`,
			argName:       "role",
			payload:       "attacker",
			expectInQuery: `"attacker"`,
		},
		{
			name:          "replace number literal",
			body:          `{"query":"query GetUsers { users(limit: 10) { id } }"}`,
			argName:       "limit",
			payload:       "99999",
			expectInQuery: "99999",
		},
		{
			name:          "replace boolean with string payload",
			body:          `{"query":"query GetUsers { users(active: true) { id } }"}`,
			argName:       "active",
			payload:       "false",
			expectInQuery: "false",
		},
		{
			name:          "replace with SQL injection payload",
			body:          `{"query":"query GetUsers { users(role: \"admin\") { id } }"}`,
			argName:       "role",
			payload:       "' OR '1'='1",
			expectInQuery: `"' OR '1'='1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := InsertionPointBuilder{
				Point: InsertionPoint{
					Type: InsertionPointTypeGraphQLInlineArg,
					Name: tt.argName,
				},
				Payload: tt.payload,
			}

			result, err := modifyGraphQLInlineArg([]byte(tt.body), []InsertionPointBuilder{builder})
			if err != nil {
				t.Fatalf("modifyGraphQLInlineArg() error: %v", err)
			}

			var parsed map[string]any
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}

			queryStr, ok := parsed["query"].(string)
			if !ok {
				t.Fatal("Result missing 'query' field")
			}

			if !contains(queryStr, tt.expectInQuery) {
				t.Errorf("Expected query to contain %q, got: %s", tt.expectInQuery, queryStr)
			}
		})
	}
}

func TestModifyGraphQLInlineArgPreservesStructure(t *testing.T) {
	body := `{"query":"mutation Test($email: String!) { createUser(email: $email, role: \"admin\", active: true) { id } }","variables":{"email":"test@test.com"},"operationName":"Test"}`

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeGraphQLInlineArg,
			Name: "role",
		},
		Payload: "injected",
	}

	result, err := modifyGraphQLInlineArg([]byte(body), []InsertionPointBuilder{builder})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if parsed["operationName"] != "Test" {
		t.Errorf("operationName was modified: got %v", parsed["operationName"])
	}

	vars, ok := parsed["variables"].(map[string]any)
	if !ok {
		t.Fatal("variables field missing")
	}
	if vars["email"] != "test@test.com" {
		t.Errorf("variables.email was modified: got %v", vars["email"])
	}

	queryStr := parsed["query"].(string)
	if !contains(queryStr, "$email") {
		t.Error("Variable reference $email was removed from query")
	}
	if !contains(queryStr, "active: true") {
		t.Error("Unrelated argument 'active' was modified")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
