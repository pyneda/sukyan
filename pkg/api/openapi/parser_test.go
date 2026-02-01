package openapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

func newTestDefinition(rawJSON string) *db.APIDefinition {
	return &db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte(rawJSON),
	}
}

func TestParse_ValidOpenAPI30(t *testing.T) {
	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users/{id}": {
				"get": {
					"operationId": "getUser",
					"summary": "Get user by ID",
					"tags": ["users"],
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true,
							"schema": {"type": "integer", "minimum": 1}
						},
						{
							"name": "fields",
							"in": "query",
							"required": false,
							"schema": {"type": "string"}
						}
					],
					"responses": {
						"200": {
							"description": "Success",
							"content": {"application/json": {"schema": {"type": "object"}}}
						}
					}
				},
				"delete": {
					"operationId": "deleteUser",
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true,
							"schema": {"type": "integer"}
						}
					],
					"responses": {"204": {"description": "Deleted"}}
				}
			},
			"/users": {
				"post": {
					"operationId": "createUser",
					"requestBody": {
						"required": true,
						"description": "User payload",
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"required": ["name", "email"],
									"properties": {
										"name":  {"type": "string", "minLength": 1, "maxLength": 100},
										"email": {"type": "string", "format": "email"},
										"age":   {"type": "integer", "minimum": 0, "maximum": 150}
									}
								}
							}
						}
					},
					"responses": {"201": {"description": "Created"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ops) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(ops))
	}

	opByID := make(map[string]core.Operation)
	for _, op := range ops {
		opByID[op.OperationID] = op
	}

	t.Run("GET /users/{id}", func(t *testing.T) {
		op, ok := opByID["getUser"]
		if !ok {
			t.Fatal("getUser operation not found")
		}
		if op.Method != "GET" {
			t.Errorf("expected method GET, got %s", op.Method)
		}
		if op.Path != "/users/{id}" {
			t.Errorf("expected path /users/{id}, got %s", op.Path)
		}
		if op.Summary != "Get user by ID" {
			t.Errorf("expected summary 'Get user by ID', got %q", op.Summary)
		}
		if op.APIType != core.APITypeOpenAPI {
			t.Errorf("expected api type openapi, got %s", op.APIType)
		}
		if op.BaseURL != "https://api.example.com" {
			t.Errorf("expected base url https://api.example.com, got %s", op.BaseURL)
		}
		if len(op.Tags) != 1 || op.Tags[0] != "users" {
			t.Errorf("expected tags [users], got %v", op.Tags)
		}
		if len(op.Parameters) != 2 {
			t.Fatalf("expected 2 parameters, got %d", len(op.Parameters))
		}

		paramsByName := make(map[string]core.Parameter)
		for _, p := range op.Parameters {
			paramsByName[p.Name] = p
		}

		idParam := paramsByName["id"]
		if idParam.Location != core.ParameterLocationPath {
			t.Errorf("expected id location path, got %s", idParam.Location)
		}
		if !idParam.Required {
			t.Error("expected id to be required")
		}
		if idParam.DataType != core.DataTypeInteger {
			t.Errorf("expected id data type integer, got %s", idParam.DataType)
		}
		if idParam.Constraints.Minimum == nil || *idParam.Constraints.Minimum != 1 {
			t.Errorf("expected id minimum constraint of 1, got %v", idParam.Constraints.Minimum)
		}

		fieldsParam := paramsByName["fields"]
		if fieldsParam.Location != core.ParameterLocationQuery {
			t.Errorf("expected fields location query, got %s", fieldsParam.Location)
		}
		if fieldsParam.Required {
			t.Error("expected fields to not be required")
		}
		if fieldsParam.DataType != core.DataTypeString {
			t.Errorf("expected fields data type string, got %s", fieldsParam.DataType)
		}
	})

	t.Run("DELETE /users/{id}", func(t *testing.T) {
		op, ok := opByID["deleteUser"]
		if !ok {
			t.Fatal("deleteUser operation not found")
		}
		if op.Method != "DELETE" {
			t.Errorf("expected method DELETE, got %s", op.Method)
		}
	})

	t.Run("POST /users request body", func(t *testing.T) {
		op, ok := opByID["createUser"]
		if !ok {
			t.Fatal("createUser operation not found")
		}
		if op.Method != "POST" {
			t.Errorf("expected method POST, got %s", op.Method)
		}
		if len(op.Parameters) != 3 {
			t.Fatalf("expected 3 body parameters, got %d", len(op.Parameters))
		}

		paramsByName := make(map[string]core.Parameter)
		for _, p := range op.Parameters {
			paramsByName[p.Name] = p
		}

		nameParam := paramsByName["name"]
		if nameParam.Location != core.ParameterLocationBody {
			t.Errorf("expected name location body, got %s", nameParam.Location)
		}
		if !nameParam.Required {
			t.Error("expected name to be required")
		}
		if nameParam.DataType != core.DataTypeString {
			t.Errorf("expected name data type string, got %s", nameParam.DataType)
		}
		if nameParam.Constraints.MinLength == nil || *nameParam.Constraints.MinLength != 1 {
			t.Errorf("expected name minLength 1, got %v", nameParam.Constraints.MinLength)
		}
		if nameParam.Constraints.MaxLength == nil || *nameParam.Constraints.MaxLength != 100 {
			t.Errorf("expected name maxLength 100, got %v", nameParam.Constraints.MaxLength)
		}

		emailParam := paramsByName["email"]
		if !emailParam.Required {
			t.Error("expected email to be required")
		}
		if emailParam.Constraints.Format != "email" {
			t.Errorf("expected email format 'email', got %q", emailParam.Constraints.Format)
		}

		ageParam := paramsByName["age"]
		if ageParam.Required {
			t.Error("expected age to not be required")
		}
		if ageParam.DataType != core.DataTypeInteger {
			t.Errorf("expected age data type integer, got %s", ageParam.DataType)
		}
		if ageParam.Constraints.Minimum == nil || *ageParam.Constraints.Minimum != 0 {
			t.Errorf("expected age minimum 0, got %v", ageParam.Constraints.Minimum)
		}
		if ageParam.Constraints.Maximum == nil || *ageParam.Constraints.Maximum != 150 {
			t.Errorf("expected age maximum 150, got %v", ageParam.Constraints.Maximum)
		}

		if op.OpenAPI == nil {
			t.Fatal("expected openapi metadata to be set")
		}
		if op.OpenAPI.RequestBody == nil {
			t.Fatal("expected request body info to be set")
		}
		if !op.OpenAPI.RequestBody.Required {
			t.Error("expected request body to be required")
		}
		if op.OpenAPI.RequestBody.ContentType != "application/json" {
			t.Errorf("expected content type application/json, got %s", op.OpenAPI.RequestBody.ContentType)
		}
	})

	t.Run("OpenAPI metadata", func(t *testing.T) {
		op := ops[0]
		if op.OpenAPI == nil {
			t.Fatal("expected openapi metadata")
		}
		if op.OpenAPI.Version != "3.0.3" {
			t.Errorf("expected version 3.0.3, got %s", op.OpenAPI.Version)
		}
		if len(op.OpenAPI.Servers) != 1 || op.OpenAPI.Servers[0] != "https://api.example.com" {
			t.Errorf("expected servers [https://api.example.com], got %v", op.OpenAPI.Servers)
		}
	})
}

func TestParse_DefinitionIDPropagated(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/ping": {
				"get": {
					"operationId": "ping",
					"responses": {"200": {"description": "pong"}}
				}
			}
		}
	}`

	defID := uuid.New()
	definition := &db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte(spec),
	}
	definition.ID = defID

	parser := NewParser()
	ops, err := parser.Parse(definition)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].DefinitionID != defID {
		t.Errorf("expected definition ID %s, got %s", defID, ops[0].DefinitionID)
	}
}

func TestParse_BaseURLFallback(t *testing.T) {
	tests := []struct {
		name            string
		baseURL         string
		serverURL       string
		expectedBaseURL string
	}{
		{
			name:            "uses definition BaseURL when set",
			baseURL:         "https://override.example.com",
			serverURL:       "https://server.example.com",
			expectedBaseURL: "https://override.example.com",
		},
		{
			name:            "falls back to server URL when BaseURL empty",
			baseURL:         "",
			serverURL:       "https://server.example.com",
			expectedBaseURL: "https://server.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := `{
				"openapi": "3.0.0",
				"info": {"title": "Test", "version": "1.0.0"},
				"servers": [{"url": "` + tt.serverURL + `"}],
				"paths": {
					"/test": {
						"get": {
							"responses": {"200": {"description": "OK"}}
						}
					}
				}
			}`

			definition := &db.APIDefinition{
				Type:          db.APIDefinitionTypeOpenAPI,
				BaseURL:       tt.baseURL,
				RawDefinition: []byte(spec),
			}

			parser := NewParser()
			ops, err := parser.Parse(definition)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ops) == 0 {
				t.Fatal("expected at least one operation")
			}
			if ops[0].BaseURL != tt.expectedBaseURL {
				t.Errorf("expected base url %q, got %q", tt.expectedBaseURL, ops[0].BaseURL)
			}
		})
	}
}

func TestParse_ErrorCases(t *testing.T) {
	tests := []struct {
		name       string
		definition *db.APIDefinition
		wantErr    string
	}{
		{
			name: "wrong definition type",
			definition: &db.APIDefinition{
				Type:          db.APIDefinitionTypeGraphQL,
				RawDefinition: []byte(`{}`),
			},
			wantErr: "expected OpenAPI definition",
		},
		{
			name: "empty raw definition",
			definition: &db.APIDefinition{
				Type:          db.APIDefinitionTypeOpenAPI,
				RawDefinition: nil,
			},
			wantErr: "empty raw definition",
		},
		{
			name: "invalid JSON",
			definition: &db.APIDefinition{
				Type:          db.APIDefinitionTypeOpenAPI,
				RawDefinition: []byte(`{not json`),
			},
			wantErr: "failed to parse OpenAPI spec",
		},
	}

	parser := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.definition)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestParse_NilPaths(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 operations for nil paths, got %d", len(ops))
	}
}

func TestParse_NilSchemaReferences(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/test": {
				"get": {
					"operationId": "nilSchemaTest",
					"parameters": [
						{
							"name": "q",
							"in": "query",
							"required": false
						}
					],
					"responses": {"200": {"description": "OK"}}
				},
				"post": {
					"operationId": "nilBodySchemaTest",
					"requestBody": {
						"content": {
							"application/json": {}
						}
					},
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}

	opByID := make(map[string]core.Operation)
	for _, op := range ops {
		opByID[op.OperationID] = op
	}

	getOp := opByID["nilSchemaTest"]
	if len(getOp.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(getOp.Parameters))
	}
	if getOp.Parameters[0].DataType != "" {
		t.Errorf("expected empty data type for nil schema, got %s", getOp.Parameters[0].DataType)
	}

	postOp := opByID["nilBodySchemaTest"]
	if len(postOp.Parameters) != 0 {
		t.Errorf("expected 0 parameters when body schema is nil, got %d", len(postOp.Parameters))
	}
}

func TestParse_NestedObjectParameters(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/orders": {
				"post": {
					"operationId": "createOrder",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"required": ["item"],
									"properties": {
										"item": {
											"type": "object",
											"properties": {
												"name":     {"type": "string"},
												"quantity": {"type": "integer", "minimum": 1},
												"tags":     {"type": "array", "items": {"type": "string"}}
											}
										},
										"note": {"type": "string"}
									}
								}
							}
						}
					},
					"responses": {"201": {"description": "Created"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	paramsByName := make(map[string]core.Parameter)
	for _, p := range ops[0].Parameters {
		paramsByName[p.Name] = p
	}

	itemParam, ok := paramsByName["item"]
	if !ok {
		t.Fatal("expected 'item' parameter")
	}
	if itemParam.DataType != core.DataTypeObject {
		t.Errorf("expected item data type object, got %s", itemParam.DataType)
	}
	if !itemParam.Required {
		t.Error("expected item to be required")
	}
	if len(itemParam.NestedParams) != 3 {
		t.Fatalf("expected 3 nested params in item, got %d", len(itemParam.NestedParams))
	}

	nestedByName := make(map[string]core.Parameter)
	for _, p := range itemParam.NestedParams {
		nestedByName[p.Name] = p
	}

	quantityParam := nestedByName["quantity"]
	if quantityParam.DataType != core.DataTypeInteger {
		t.Errorf("expected quantity data type integer, got %s", quantityParam.DataType)
	}
	if quantityParam.Constraints.Minimum == nil || *quantityParam.Constraints.Minimum != 1 {
		t.Errorf("expected quantity minimum 1, got %v", quantityParam.Constraints.Minimum)
	}

	tagsParam := nestedByName["tags"]
	if tagsParam.DataType != core.DataTypeArray {
		t.Errorf("expected tags data type array, got %s", tagsParam.DataType)
	}
	if len(tagsParam.NestedParams) != 1 {
		t.Fatalf("expected 1 nested param (items) in tags, got %d", len(tagsParam.NestedParams))
	}
	if tagsParam.NestedParams[0].Name != "items" {
		t.Errorf("expected nested param name 'items', got %q", tagsParam.NestedParams[0].Name)
	}
	if tagsParam.NestedParams[0].DataType != core.DataTypeString {
		t.Errorf("expected items data type string, got %s", tagsParam.NestedParams[0].DataType)
	}

	noteParam := paramsByName["note"]
	if noteParam.DataType != core.DataTypeString {
		t.Errorf("expected note data type string, got %s", noteParam.DataType)
	}
	if noteParam.Required {
		t.Error("expected note to not be required")
	}
}

func TestParse_CircularReferenceProtection(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/nodes": {
				"post": {
					"operationId": "createNode",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {"$ref": "#/components/schemas/Node"}
							}
						}
					},
					"responses": {"200": {"description": "OK"}}
				}
			}
		},
		"components": {
			"schemas": {
				"Node": {
					"type": "object",
					"properties": {
						"value": {"type": "string"},
						"child": {"$ref": "#/components/schemas/Node"}
					}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	var totalParams func(params []core.Parameter) int
	totalParams = func(params []core.Parameter) int {
		count := len(params)
		for _, p := range params {
			count += totalParams(p.NestedParams)
		}
		return count
	}

	total := totalParams(ops[0].Parameters)
	if total > 100 {
		t.Errorf("circular reference protection may have failed: total params %d exceeds reasonable limit", total)
	}
}

func TestParse_DeepNestingDepthLimit(t *testing.T) {
	totalLevels := 15
	schemas := make(map[string]any)
	for i := 0; i < totalLevels; i++ {
		name := fmt.Sprintf("Level%d", i)
		if i < totalLevels-1 {
			nextName := fmt.Sprintf("Level%d", i+1)
			schemas[name] = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"child": map[string]any{"$ref": "#/components/schemas/" + nextName},
				},
			}
		} else {
			schemas[name] = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"leaf": map[string]any{"type": "string"},
				},
			}
		}
	}

	spec := map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]any{"title": "Deep Nesting", "version": "1.0.0"},
		"paths": map[string]any{
			"/deep": map[string]any{
				"post": map[string]any{
					"operationId": "deepNest",
					"requestBody": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Level0"},
							},
						},
					},
					"responses": map[string]any{"200": map[string]any{"description": "OK"}},
				},
			},
		},
		"components": map[string]any{
			"schemas": schemas,
		},
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(string(specJSON)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	depth := maxNestedDepth(ops[0].Parameters)
	// extractSchemaInfoWithDepth stops when depth > maxSchemaDepth (10).
	// parseRequestBody starts extraction at depth 0 for each top-level property,
	// so the effective nesting measured from the parameter tree is maxSchemaDepth + 2
	// (body property level + 0..maxSchemaDepth inclusive range).
	// The key assertion: depth must be less than totalLevels, proving truncation occurred.
	if depth >= totalLevels {
		t.Errorf("depth %d reached totalLevels %d, depth limiting failed", depth, totalLevels)
	}
	if depth < maxSchemaDepth {
		t.Errorf("depth %d is suspiciously low, expected at least %d", depth, maxSchemaDepth)
	}
}

func TestParse_SecurityRequirements(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"security": [{"bearerAuth": []}],
		"components": {
			"securitySchemes": {
				"bearerAuth": {
					"type": "http",
					"scheme": "bearer"
				},
				"apiKey": {
					"type": "apiKey",
					"name": "X-API-Key",
					"in": "header"
				}
			}
		},
		"paths": {
			"/public": {
				"get": {
					"operationId": "publicEndpoint",
					"security": [],
					"responses": {"200": {"description": "OK"}}
				}
			},
			"/protected": {
				"get": {
					"operationId": "protectedEndpoint",
					"security": [{"apiKey": []}],
					"responses": {"200": {"description": "OK"}}
				}
			},
			"/default-security": {
				"get": {
					"operationId": "defaultSecurity",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opByID := make(map[string]core.Operation)
	for _, op := range ops {
		opByID[op.OperationID] = op
	}

	t.Run("operation-level override with empty security", func(t *testing.T) {
		op := opByID["publicEndpoint"]
		if len(op.Security) != 0 {
			t.Errorf("expected 0 security requirements for public endpoint, got %d", len(op.Security))
		}
	})

	t.Run("operation-level security override", func(t *testing.T) {
		op := opByID["protectedEndpoint"]
		if len(op.Security) != 1 {
			t.Fatalf("expected 1 security requirement, got %d", len(op.Security))
		}
		if op.Security[0].Name != "apiKey" {
			t.Errorf("expected security name 'apiKey', got %q", op.Security[0].Name)
		}
		if op.Security[0].Type != "apiKey" {
			t.Errorf("expected security type 'apiKey', got %q", op.Security[0].Type)
		}
	})

	t.Run("global security fallback", func(t *testing.T) {
		op := opByID["defaultSecurity"]
		if len(op.Security) != 1 {
			t.Fatalf("expected 1 security requirement from global, got %d", len(op.Security))
		}
		if op.Security[0].Name != "bearerAuth" {
			t.Errorf("expected security name 'bearerAuth', got %q", op.Security[0].Name)
		}
		if op.Security[0].Type != "http" {
			t.Errorf("expected security type 'http', got %q", op.Security[0].Type)
		}
	})
}

func TestParse_ContentTypes(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/upload": {
				"post": {
					"operationId": "upload",
					"requestBody": {
						"content": {
							"multipart/form-data": {
								"schema": {"type": "object", "properties": {"file": {"type": "string", "format": "binary"}}}
							}
						}
					},
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {"schema": {"type": "object"}}
							}
						}
					}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}

	ct := ops[0].ContentTypes
	if len(ct.Request) != 1 || ct.Request[0] != "multipart/form-data" {
		t.Errorf("expected request content type [multipart/form-data], got %v", ct.Request)
	}
	if len(ct.Response) != 1 || ct.Response[0] != "application/json" {
		t.Errorf("expected response content type [application/json], got %v", ct.Response)
	}
}

func TestParse_ParameterConstraints(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/search": {
				"get": {
					"operationId": "search",
					"parameters": [
						{
							"name": "q",
							"in": "query",
							"required": true,
							"schema": {
								"type": "string",
								"minLength": 3,
								"maxLength": 200,
								"pattern": "^[a-zA-Z0-9 ]+$"
							}
						},
						{
							"name": "page",
							"in": "query",
							"schema": {
								"type": "integer",
								"minimum": 1,
								"maximum": 1000,
								"default": 1
							}
						},
						{
							"name": "sort",
							"in": "query",
							"schema": {
								"type": "string",
								"enum": ["asc", "desc"]
							}
						},
						{
							"name": "tags",
							"in": "query",
							"schema": {
								"type": "array",
								"items": {"type": "string"},
								"minItems": 1,
								"maxItems": 10
							}
						}
					],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	paramsByName := make(map[string]core.Parameter)
	for _, p := range ops[0].Parameters {
		paramsByName[p.Name] = p
	}

	t.Run("string constraints", func(t *testing.T) {
		q := paramsByName["q"]
		if q.Constraints.MinLength == nil || *q.Constraints.MinLength != 3 {
			t.Errorf("expected minLength 3, got %v", q.Constraints.MinLength)
		}
		if q.Constraints.MaxLength == nil || *q.Constraints.MaxLength != 200 {
			t.Errorf("expected maxLength 200, got %v", q.Constraints.MaxLength)
		}
		if q.Constraints.Pattern != "^[a-zA-Z0-9 ]+$" {
			t.Errorf("expected pattern '^[a-zA-Z0-9 ]+$', got %q", q.Constraints.Pattern)
		}
	})

	t.Run("integer constraints with default", func(t *testing.T) {
		page := paramsByName["page"]
		if page.Constraints.Minimum == nil || *page.Constraints.Minimum != 1 {
			t.Errorf("expected minimum 1, got %v", page.Constraints.Minimum)
		}
		if page.Constraints.Maximum == nil || *page.Constraints.Maximum != 1000 {
			t.Errorf("expected maximum 1000, got %v", page.Constraints.Maximum)
		}
		if page.DefaultValue == nil {
			t.Error("expected default value to be set")
		}
	})

	t.Run("enum constraints", func(t *testing.T) {
		sort := paramsByName["sort"]
		if len(sort.Constraints.Enum) != 2 {
			t.Fatalf("expected 2 enum values, got %d", len(sort.Constraints.Enum))
		}
	})

	t.Run("array constraints", func(t *testing.T) {
		tags := paramsByName["tags"]
		if tags.DataType != core.DataTypeArray {
			t.Errorf("expected data type array, got %s", tags.DataType)
		}
		if tags.Constraints.MinItems == nil || *tags.Constraints.MinItems != 1 {
			t.Errorf("expected minItems 1, got %v", tags.Constraints.MinItems)
		}
		if tags.Constraints.MaxItems == nil || *tags.Constraints.MaxItems != 10 {
			t.Errorf("expected maxItems 10, got %v", tags.Constraints.MaxItems)
		}
	})
}

func TestParse_DeprecatedOperationAndParameter(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/old": {
				"get": {
					"operationId": "deprecated",
					"deprecated": true,
					"parameters": [
						{
							"name": "old_param",
							"in": "query",
							"deprecated": true,
							"schema": {"type": "string"}
						}
					],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ops[0].Deprecated {
		t.Error("expected operation to be deprecated")
	}
	if !ops[0].Parameters[0].Deprecated {
		t.Error("expected parameter to be deprecated")
	}
}

func TestParse_NonObjectRequestBody(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/raw": {
				"post": {
					"operationId": "rawBody",
					"requestBody": {
						"required": true,
						"content": {
							"text/plain": {
								"schema": {"type": "string"}
							}
						}
					},
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops[0].Parameters) != 1 {
		t.Fatalf("expected 1 body parameter, got %d", len(ops[0].Parameters))
	}
	param := ops[0].Parameters[0]
	if param.Name != "body" {
		t.Errorf("expected param name 'body', got %q", param.Name)
	}
	if param.DataType != core.DataTypeString {
		t.Errorf("expected data type string, got %s", param.DataType)
	}
	if param.Location != core.ParameterLocationBody {
		t.Errorf("expected location body, got %s", param.Location)
	}
	if !param.Required {
		t.Error("expected body param to be required")
	}
	if param.ContentType != "text/plain" {
		t.Errorf("expected content type 'text/plain', got %q", param.ContentType)
	}
}

func TestMapLocation(t *testing.T) {
	tests := []struct {
		input    string
		expected core.ParameterLocation
	}{
		{"path", core.ParameterLocationPath},
		{"query", core.ParameterLocationQuery},
		{"header", core.ParameterLocationHeader},
		{"cookie", core.ParameterLocationCookie},
		{"body", core.ParameterLocationBody},
		{"unknown", core.ParameterLocationQuery},
		{"", core.ParameterLocationQuery},
	}

	p := NewParser()
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := p.mapLocation(tt.input)
			if result != tt.expected {
				t.Errorf("mapLocation(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMapDataType(t *testing.T) {
	tests := []struct {
		input    string
		expected core.DataType
	}{
		{"string", core.DataTypeString},
		{"integer", core.DataTypeInteger},
		{"number", core.DataTypeNumber},
		{"boolean", core.DataTypeBoolean},
		{"array", core.DataTypeArray},
		{"object", core.DataTypeObject},
		{"file", core.DataTypeFile},
		{"unknown", core.DataTypeString},
		{"", core.DataTypeString},
	}

	p := NewParser()
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := p.mapDataType(tt.input)
			if result != tt.expected {
				t.Errorf("mapDataType(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsPropertyRequired(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		propName string
		required []string
		expected bool
	}{
		{"in required list", "name", []string{"name", "email"}, true},
		{"not in required list", "age", []string{"name", "email"}, false},
		{"empty required list", "name", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.isPropertyRequired(tt.propName, tt.required)
			if result != tt.expected {
				t.Errorf("isPropertyRequired(%q, %v) = %v, want %v", tt.propName, tt.required, result, tt.expected)
			}
		})
	}
}

func TestExtractConstraintsFromSchema(t *testing.T) {
	schemaJSON := `{
		"type": "string",
		"minLength": 5,
		"maxLength": 50,
		"pattern": "^[a-z]+$",
		"format": "hostname"
	}`

	constraints, err := ExtractConstraintsFromSchema([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if constraints.MinLength == nil || *constraints.MinLength != 5 {
		t.Errorf("expected minLength 5, got %v", constraints.MinLength)
	}
	if constraints.MaxLength == nil || *constraints.MaxLength != 50 {
		t.Errorf("expected maxLength 50, got %v", constraints.MaxLength)
	}
	if constraints.Pattern != "^[a-z]+$" {
		t.Errorf("expected pattern '^[a-z]+$', got %q", constraints.Pattern)
	}
	if constraints.Format != "hostname" {
		t.Errorf("expected format 'hostname', got %q", constraints.Format)
	}
}

func TestExtractConstraintsFromSchema_InvalidJSON(t *testing.T) {
	_, err := ExtractConstraintsFromSchema([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseFromRawDefinition(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/health": {
				"get": {
					"operationId": "health",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	ops, err := ParseFromRawDefinition([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].OperationID != "health" {
		t.Errorf("expected operation ID 'health', got %q", ops[0].OperationID)
	}
}

func TestParse_ParameterLocationMapping(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {
			"/test/{id}": {
				"get": {
					"operationId": "locationTest",
					"parameters": [
						{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}},
						{"name": "q", "in": "query", "schema": {"type": "string"}},
						{"name": "X-Request-Id", "in": "header", "schema": {"type": "string"}},
						{"name": "session", "in": "cookie", "schema": {"type": "string"}}
					],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	parser := NewParser()
	ops, err := parser.Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]core.ParameterLocation{
		"id":           core.ParameterLocationPath,
		"q":            core.ParameterLocationQuery,
		"X-Request-Id": core.ParameterLocationHeader,
		"session":      core.ParameterLocationCookie,
	}

	paramsByName := make(map[string]core.Parameter)
	for _, p := range ops[0].Parameters {
		paramsByName[p.Name] = p
	}

	for name, expectedLocation := range expected {
		param, ok := paramsByName[name]
		if !ok {
			t.Errorf("parameter %q not found", name)
			continue
		}
		if param.Location != expectedLocation {
			t.Errorf("parameter %q: expected location %s, got %s", name, expectedLocation, param.Location)
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

func maxNestedDepth(params []core.Parameter) int {
	if len(params) == 0 {
		return 0
	}
	max := 0
	for _, p := range params {
		d := maxNestedDepth(p.NestedParams)
		if d > max {
			max = d
		}
	}
	return max + 1
}
