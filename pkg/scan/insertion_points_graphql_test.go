package scan

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
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
			name:     "minified query with variable definitions",
			body:     `{"query": "query($id:ID!){user(id:$id){name}}"}`,
			expected: true,
		},
		{
			name:     "minified mutation with variable definitions",
			body:     `{"query": "mutation($name:String!){createUser(name:$name){id}}"}`,
			expected: true,
		},
		{
			name:     "minified subscription with variable definitions",
			body:     `{"query": "subscription($room:ID!){messageAdded(room:$room){text}}"}`,
			expected: true,
		},
		{
			name:     "named operation without space before selection set",
			body:     `{"query": "query GetUser{user{name}}"}`,
			expected: true,
		},
		{
			name:     "keyword immediately followed by selection set",
			body:     `{"query": "query{users{id}}"}`,
			expected: true,
		},
		{
			name:     "keyword followed by newline",
			body:     "{\"query\": \"query\\n  GetUser {\\n    user { name }\\n  }\"}",
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
			name:     "search term starting with the query keyword",
			body:     `{"query": "queryable products", "page": 1}`,
			expected: false,
		},
		{
			name:     "search term with no selection set",
			body:     `{"query": "query results for running shoes"}`,
			expected: false,
		},
		{
			name:     "search term starting with the mutation keyword",
			body:     `{"query": "mutation testing tools"}`,
			expected: false,
		},
		{
			name:     "prose search term",
			body:     `{"query": "red running shoes", "page": 1}`,
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
			point := inlineArgPoint(t, tt.body, tt.argName)
			queryStr := rebuildInlineArg(t, tt.body, point, tt.payload)

			if !contains(queryStr, tt.expectInQuery) {
				t.Errorf("Expected query to contain %q, got: %s", tt.expectInQuery, queryStr)
			}
		})
	}
}

func TestModifyGraphQLInlineArgPreservesStructure(t *testing.T) {
	body := `{"query":"mutation Test($email: String!) { createUser(email: $email, role: \"admin\", active: true) { id } }","variables":{"email":"test@test.com"},"operationName":"Test"}`

	point := inlineArgPoint(t, body, "role")

	result, err := modifyGraphQLInlineArg([]byte(body), []InsertionPointBuilder{{Point: point, Payload: "injected"}})
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

// A GraphQL envelope's top-level fields are protocol structure: replacing query,
// variables, operationName or the whole body makes the server answer 400 before any
// resolver runs, so those points can never produce a finding. Only the GraphQL
// variable/inline-argument points reach application code.
func TestGetInsertionPointsGraphQLEnvelope(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantGraphQL    []string
		wantInlineArgs []string
		wantBody       []string
		wantFullBody   bool
	}{
		{
			name:        "graphql operation with variables",
			body:        `{"query":"query GetUser($id: ID!){user(id:$id){name}}","variables":{"id":"1"},"operationName":"GetUser"}`,
			wantGraphQL: []string{"id"},
		},
		{
			name:        "minified graphql operation",
			body:        `{"query":"query($id:ID!){user(id:$id){name}}","variables":{"id":"1"}}`,
			wantGraphQL: []string{"id"},
		},
		{
			name:           "shorthand operation with inline argument",
			body:           `{"query":"{user(id:\"42\"){name}}"}`,
			wantInlineArgs: []string{"id"},
		},
		{
			name: "introspection without variables or inline arguments",
			body: `{"query":"query IntrospectionQuery{__schema{types{name}}}"}`,
		},
		{
			name:         "plain json body",
			body:         `{"name":"test","email":"test@example.com"}`,
			wantBody:     []string{"name", "email"},
			wantFullBody: true,
		},
		{
			name:         "search api using a query field",
			body:         `{"query":"red running shoes","page":1}`,
			wantBody:     []string{"query", "page"},
			wantFullBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &db.History{
				URL:                "http://example.com/graphql",
				Method:             "POST",
				RawRequest:         []byte("POST /graphql HTTP/1.1\r\nContent-Type: application/json\r\n\r\n" + tt.body),
				RequestContentType: "application/json",
			}

			points, err := GetInsertionPoints(history, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertPointNames(t, points, InsertionPointTypeGraphQLVariable, tt.wantGraphQL)
			assertPointNames(t, points, InsertionPointTypeGraphQLInlineArg, tt.wantInlineArgs)
			assertPointNames(t, points, InsertionPointTypeBody, tt.wantBody)

			fullBody := pointNames(points, InsertionPointTypeFullBody)
			if tt.wantFullBody && len(fullBody) != 1 {
				t.Errorf("expected one fullbody point, got %v", fullBody)
			}
			if !tt.wantFullBody && len(fullBody) != 0 {
				t.Errorf("expected no fullbody point, got %v", fullBody)
			}
		})
	}
}

func assertPointNames(t *testing.T, points []InsertionPoint, pointType InsertionPointType, want []string) {
	t.Helper()

	got := pointNames(points, pointType)
	if len(got) != len(want) {
		t.Fatalf("%s points: expected %v, got %v", pointType, want, got)
	}
	for _, name := range want {
		if !hasPoint(points, pointType, name) {
			t.Errorf("%s point %q missing, got %v", pointType, name, got)
		}
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

func graphQLBody(t *testing.T, query string) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("failed to build GraphQL body: %v", err)
	}
	return string(body)
}

func inlineArgPoints(t *testing.T, body string) []InsertionPoint {
	t.Helper()

	var envelope map[string]any
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("failed to parse GraphQL body: %v", err)
	}
	query, ok := envelope["query"].(string)
	if !ok {
		t.Fatal("GraphQL body missing 'query'")
	}
	return extractGraphQLInlineArgPoints(query, body)
}

func inlineArgPoint(t *testing.T, body string, name string) InsertionPoint {
	t.Helper()

	points := inlineArgPoints(t, body)
	for _, p := range points {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("inline arg point %q not found, got %v", name, inlineArgPointNames(points))
	return InsertionPoint{}
}

func inlineArgPointNames(points []InsertionPoint) []string {
	names := make([]string, 0, len(points))
	for _, p := range points {
		names = append(names, p.Name)
	}
	return names
}

// rebuildInlineArg injects payload at point and returns the resulting query, failing
// the test when the rebuilt document is not parseable GraphQL. A payload that breaks
// the document is never tested by the server, so parseability is a hard requirement
// of every rewrite.
func rebuildInlineArg(t *testing.T, body string, point InsertionPoint, payload string) string {
	t.Helper()

	result, err := modifyGraphQLInlineArg([]byte(body), []InsertionPointBuilder{{Point: point, Payload: payload}})
	if err != nil {
		t.Fatalf("modifyGraphQLInlineArg() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse rebuilt body: %v", err)
	}
	query, ok := parsed["query"].(string)
	if !ok {
		t.Fatal("rebuilt body missing 'query'")
	}
	if _, err := parser.ParseQuery(&ast.Source{Input: query}); err != nil {
		t.Fatalf("rebuilt query is not parseable GraphQL: %v\nquery: %s", err, query)
	}
	return query
}

// An inline argument sharing its name with a declared variable must be rewritten at
// the argument, never in the operation's variable definition list: writing there
// makes the whole document unparseable and leaves the real sink untested.
func TestModifyGraphQLInlineArgSkipsVariableDefinitions(t *testing.T) {
	body := graphQLBody(t, `query GetUsers($role: String) { users(limit: 10, role: "admin") { id } }`)

	tests := []struct {
		pointName string
		payload   string
		want      string
	}{
		{
			pointName: "role",
			payload:   "PWN",
			want:      `query GetUsers($role: String) { users(limit: 10, role: "PWN") { id } }`,
		},
		{
			pointName: "limit",
			payload:   "PWN",
			want:      `query GetUsers($role: String) { users(limit: "PWN", role: "admin") { id } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.pointName, func(t *testing.T) {
			point := inlineArgPoint(t, body, tt.pointName)
			if got := rebuildInlineArg(t, body, point, tt.payload); got != tt.want {
				t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

// Quoting follows the original literal's syntactic form, not the payload's shape: a
// bare payload in a string position and a quoted payload in an enum position are both
// rejected before any resolver runs.
func TestModifyGraphQLInlineArgPreservesLiteralForm(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		pointName string
		payload   string
		want      string
	}{
		{
			name:      "string keeps quotes",
			query:     `query Test { users(role: "admin") { id } }`,
			pointName: "role",
			payload:   "' OR '1'='1",
			want:      `query Test { users(role: "' OR '1'='1") { id } }`,
		},
		{
			name:      "numeric payload stays bare in a number position",
			query:     `query Test { users(limit: 10) { id } }`,
			pointName: "limit",
			payload:   "99999",
			want:      `query Test { users(limit: 99999) { id } }`,
		},
		{
			name:      "non numeric payload is quoted in a number position",
			query:     `query Test { users(limit: 10) { id } }`,
			pointName: "limit",
			payload:   "1' OR '1'='1",
			want:      `query Test { users(limit: "1' OR '1'='1") { id } }`,
		},
		{
			name:      "name payload stays bare in an enum position",
			query:     `query Test { users(status: ACTIVE) { id } }`,
			pointName: "status",
			payload:   "SUSPENDED",
			want:      `query Test { users(status: SUSPENDED) { id } }`,
		},
		{
			name:      "non name payload is quoted in an enum position",
			query:     `query Test { users(status: ACTIVE) { id } }`,
			pointName: "status",
			payload:   "x'",
			want:      `query Test { users(status: "x'") { id } }`,
		},
		{
			name:      "boolean payload stays bare in a boolean position",
			query:     `query Test { users(active: true) { id } }`,
			pointName: "active",
			payload:   "false",
			want:      `query Test { users(active: false) { id } }`,
		},
		{
			name:      "non boolean payload is quoted in a boolean position",
			query:     `query Test { users(active: true) { id } }`,
			pointName: "active",
			payload:   "1 OR 1=1",
			want:      `query Test { users(active: "1 OR 1=1") { id } }`,
		},
		{
			name:      "null position accepts a null payload bare",
			query:     `query Test { users(deletedAt: null) { id } }`,
			pointName: "deletedAt",
			payload:   "null",
			want:      `query Test { users(deletedAt: null) { id } }`,
		},
		{
			name:      "payload with a newline is escaped inside a string literal",
			query:     `query Test { users(role: "admin") { id } }`,
			pointName: "role",
			payload:   "a\nb",
			want:      `query Test { users(role: "a\nb") { id } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := graphQLBody(t, tt.query)
			point := inlineArgPoint(t, body, tt.pointName)
			if got := rebuildInlineArg(t, body, point, tt.payload); got != tt.want {
				t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

func TestModifyGraphQLInlineArgEscapesStringPayloads(t *testing.T) {
	body := graphQLBody(t, `query Test { search(query: "hello") { id } }`)
	point := inlineArgPoint(t, body, "query")

	want := `query Test { search(query: "a\"b\\c") { id } }`
	if got := rebuildInlineArg(t, body, point, `a"b\c`); got != want {
		t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, want)
	}
}

// List and input-object arguments carry the highest-value mutation sinks; each scalar
// leaf needs its own point or none of them is ever reached.
func TestExtractGraphQLInlineArgPointsNestedValues(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantNames  []string
		wantValues []string
	}{
		{
			name:       "input object",
			query:      `mutation { createUser(input: {name:"x", role:"admin"}) { id } }`,
			wantNames:  []string{"input.name", "input.role"},
			wantValues: []string{"x", "admin"},
		},
		{
			name:       "list of scalars",
			query:      `query Test { search(tags: ["alpha", "beta"]) { id } }`,
			wantNames:  []string{"tags[0]", "tags[1]"},
			wantValues: []string{"alpha", "beta"},
		},
		{
			name:       "nested input object inside a list",
			query:      `mutation { bulk(items: [{name: "first", meta: {tag: "t1"}}, {name: "second"}]) { id } }`,
			wantNames:  []string{"items[0].name", "items[0].meta.tag", "items[1].name"},
			wantValues: []string{"first", "t1", "second"},
		},
		{
			name:       "input object mixing literals and variable references",
			query:      `mutation Test($id: ID!) { update(input: {id: $id, name: "x", limit: 5}) { id } }`,
			wantNames:  []string{"input.name", "input.limit"},
			wantValues: []string{"x", "5"},
		},
		{
			name:       "input object alongside a scalar argument",
			query:      `query Test { issues(filter: {state: "open"}, first: 10) { id } }`,
			wantNames:  []string{"filter.state", "first"},
			wantValues: []string{"open", "10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := graphQLBody(t, tt.query)
			points := inlineArgPoints(t, body)

			if len(points) != len(tt.wantNames) {
				t.Fatalf("expected points %v, got %v", tt.wantNames, inlineArgPointNames(points))
			}
			for i, p := range points {
				if p.Name != tt.wantNames[i] {
					t.Errorf("point %d: name = %q, want %q", i, p.Name, tt.wantNames[i])
				}
				if p.Value != tt.wantValues[i] {
					t.Errorf("point %d (%s): value = %q, want %q", i, p.Name, p.Value, tt.wantValues[i])
				}
			}
		})
	}
}

func TestModifyGraphQLInlineArgNestedValues(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		pointName string
		payload   string
		want      string
	}{
		{
			name:      "input object leaf",
			query:     `mutation { createUser(input: {name:"x", role:"admin"}) { id } }`,
			pointName: "input.role",
			payload:   "' OR '1'='1",
			want:      `mutation { createUser(input: {name:"x", role:"' OR '1'='1"}) { id } }`,
		},
		{
			name:      "list element",
			query:     `query Test { search(tags: ["alpha", "beta"]) { id } }`,
			pointName: "tags[1]",
			payload:   "PWN",
			want:      `query Test { search(tags: ["alpha", "PWN"]) { id } }`,
		},
		{
			name:      "nested object inside a list",
			query:     `mutation { bulk(items: [{name: "first"}, {name: "second"}]) { id } }`,
			pointName: "items[1].name",
			payload:   "PWN",
			want:      `mutation { bulk(items: [{name: "first"}, {name: "PWN"}]) { id } }`,
		},
		{
			name:      "numeric leaf inside an input object",
			query:     `mutation Test { update(input: {name: "x", limit: 5}) { id } }`,
			pointName: "input.limit",
			payload:   "PWN",
			want:      `mutation Test { update(input: {name: "x", limit: "PWN"}) { id } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := graphQLBody(t, tt.query)
			point := inlineArgPoint(t, body, tt.pointName)
			if got := rebuildInlineArg(t, body, point, tt.payload); got != tt.want {
				t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}

// Arguments repeated across sibling fields must be independently addressable and
// distinguishably named, otherwise every duplicate is a request spent re-testing the
// first field and a finding attributed to the wrong one.
func TestGraphQLInlineArgDuplicateNames(t *testing.T) {
	body := graphQLBody(t, `{ a(id: 1) b(id: 2) }`)

	points := inlineArgPoints(t, body)
	wantNames := []string{"a.id", "b.id"}
	if got := inlineArgPointNames(points); len(got) != 2 || got[0] != wantNames[0] || got[1] != wantNames[1] {
		t.Fatalf("point names = %v, want %v", got, wantNames)
	}

	wantQueries := []string{`{ a(id: "PWN") b(id: 2) }`, `{ a(id: 1) b(id: "PWN") }`}
	for i, point := range points {
		if got := rebuildInlineArg(t, body, point, "PWN"); got != wantQueries[i] {
			t.Errorf("point %s rebuilt query =\n  %s\nwant\n  %s", point.Name, got, wantQueries[i])
		}
	}
}

func TestGraphQLInlineArgDuplicateNamesAcrossNesting(t *testing.T) {
	body := graphQLBody(t, `mutation { one(input: {q: "a"}) { id } two(input: {q: "b"}) { id } }`)

	points := inlineArgPoints(t, body)
	wantNames := []string{"one.input.q", "two.input.q"}
	if got := inlineArgPointNames(points); len(got) != 2 || got[0] != wantNames[0] || got[1] != wantNames[1] {
		t.Fatalf("point names = %v, want %v", got, wantNames)
	}

	want := `mutation { one(input: {q: "a"}) { id } two(input: {q: "PWN"}) { id } }`
	if got := rebuildInlineArg(t, body, points[1], "PWN"); got != want {
		t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, want)
	}
}

// Sibling fields that differ in their arguments must be aliased in valid GraphQL, so
// the alias - not the repeated field name - is what tells a finding apart.
func TestGraphQLInlineArgDuplicateNamesUseAliases(t *testing.T) {
	body := graphQLBody(t, `query D { a: projects(workspaceId: "1") { id } b: projects(workspaceId: "2") { id } }`)

	points := inlineArgPoints(t, body)
	wantNames := []string{"a.workspaceId", "b.workspaceId"}
	if got := inlineArgPointNames(points); len(got) != 2 || got[0] != wantNames[0] || got[1] != wantNames[1] {
		t.Fatalf("point names = %v, want %v", got, wantNames)
	}

	want := `query D { a: projects(workspaceId: "1") { id } b: projects(workspaceId: "PWN") { id } }`
	if got := rebuildInlineArg(t, body, points[1], "PWN"); got != want {
		t.Errorf("rebuilt query =\n  %s\nwant\n  %s", got, want)
	}
}

func TestExtractGraphQLInlineArgPointsSkipsVariableReferences(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "argument is a variable reference",
			query: `query M($role: String) { members(role: $role) { id } }`,
		},
		{
			name:  "variable reference inside an input object",
			query: `mutation M($id: ID!) { update(input: {id: $id}) { ok } }`,
		},
		{
			name:  "variable reference inside a list",
			query: `query M($id: ID!) { items(ids: [$id]) { id } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := graphQLBody(t, tt.query)
			if points := inlineArgPoints(t, body); len(points) != 0 {
				t.Errorf("expected no inline arg points, got %v", inlineArgPointNames(points))
			}
		})
	}
}

// Recursing into list and input-object literals is what makes a hostile or merely
// huge document able to multiply a request's traffic, so both the nesting depth and
// the point count stay bounded.
func TestExtractGraphQLInlineArgPointsBounded(t *testing.T) {
	t.Run("point count is capped", func(t *testing.T) {
		var fields []string
		for i := 0; i < maxGraphQLInlineArgPoints*3; i++ {
			fields = append(fields, fmt.Sprintf(`f%d: "v%d"`, i, i))
		}
		body := graphQLBody(t, fmt.Sprintf(`mutation { create(input: {%s}) { id } }`, strings.Join(fields, ", ")))

		if points := inlineArgPoints(t, body); len(points) != maxGraphQLInlineArgPoints {
			t.Errorf("got %d points, want the %d cap", len(points), maxGraphQLInlineArgPoints)
		}
	})

	t.Run("nesting depth is bounded", func(t *testing.T) {
		value := `"leaf"`
		for i := 0; i < maxGraphQLValueDepth*5; i++ {
			value = fmt.Sprintf(`{n%d: %s}`, i, value)
		}
		body := graphQLBody(t, fmt.Sprintf(`mutation { create(input: %s, tail: "reachable") { id } }`, value))

		// The leaf sits past the cap so it yields nothing, but bailing out has to
		// consume the value it gave up on rather than abandon the parse: the argument
		// after it must still be found.
		points := inlineArgPoints(t, body)
		if got := inlineArgPointNames(points); len(got) != 1 || got[0] != "tail" {
			t.Errorf("point names = %v, want [tail]", got)
		}
	})
}

func TestExtractGraphQLInlineArgPointsValueTypes(t *testing.T) {
	body := graphQLBody(t, `query Test { items(name: "abc", id: "10", limit: 10, ratio: 1.5, active: true, status: ACTIVE, deletedAt: null) { id } }`)

	want := map[string]lib.DataType{
		"name":      lib.TypeString,
		"id":        lib.TypeString,
		"limit":     lib.TypeInt,
		"ratio":     lib.TypeFloat,
		"active":    lib.TypeBoolean,
		"status":    lib.TypeString,
		"deletedAt": lib.TypeNull,
	}

	for _, point := range inlineArgPoints(t, body) {
		expected, ok := want[point.Name]
		if !ok {
			t.Errorf("unexpected point %q", point.Name)
			continue
		}
		if point.ValueType != expected {
			t.Errorf("point %q: value type = %s, want %s", point.Name, point.ValueType, expected)
		}
		delete(want, point.Name)
	}
	for name := range want {
		t.Errorf("point %q missing", name)
	}
}

// Every point a query yields has to survive a round trip: the payload must land at
// that point and nowhere else, and the document must still parse.
func TestModifyGraphQLInlineArgAllPointsStayParseable(t *testing.T) {
	queries := []string{
		`query GetUsers($role: String) { users(limit: 10, role: "admin") { id } }`,
		`mutation { createUser(input: {name:"x", role:"admin", age: 30, admin: false}) { id } }`,
		`{ a(id: 1) b(id: 2) }`,
		`query Test { issues(filter: {state: "open", q: "boom"}, first: 10) { id } }`,
		`mutation { bulk(items: [{name: "first"}, {name: "second"}], dry: true) { id } }`,
		`query Test { users(status: ACTIVE) @include(if: true) { id } }`,
	}
	payloads := []string{"PWN", "' OR '1'='1", `"; DROP TABLE users;--`, "{{7*7}}", "../../etc/passwd", "a\nb"}

	for _, query := range queries {
		body := graphQLBody(t, query)
		points := inlineArgPoints(t, body)
		if len(points) == 0 {
			t.Errorf("query yielded no inline arg points: %s", query)
			continue
		}
		for _, point := range points {
			for _, payload := range payloads {
				rebuilt := rebuildInlineArg(t, body, point, payload)
				if rebuilt == query {
					t.Errorf("point %q with payload %q left the query unchanged: %s", point.Name, payload, query)
				}
			}
		}
	}
}
