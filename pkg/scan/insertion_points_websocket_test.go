package scan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
)

func TestGetWebSocketMessageInsertionPoints(t *testing.T) {
	tests := []struct {
		name           string
		message        *db.WebSocketMessage
		scoped         []string
		expectedCount  int
		expectTypes    []InsertionPointType
		shouldNotError bool
	}{
		{
			name: "Raw message",
			message: &db.WebSocketMessage{
				PayloadData: "Hello, World!",
			},
			scoped:         []string{"ws_raw"},
			expectedCount:  1,
			expectTypes:    []InsertionPointType{InsertionPointTypeWSRawMessage},
			shouldNotError: true,
		},
		{
			name: "JSON Object",
			message: &db.WebSocketMessage{
				PayloadData: `{"user": "john", "role": "admin", "auth": {"token": "abc123"}}`,
			},
			scoped:         []string{"ws_json"},
			expectedCount:  12, // 1 object + 3 keys + 3 values + 3 fields + 1 nested object + 1 nested field
			expectTypes:    []InsertionPointType{InsertionPointTypeWSJSONObject, InsertionPointTypeWSJSONKey, InsertionPointTypeWSJSONValue, InsertionPointTypeWSJSONField},
			shouldNotError: true,
		},
		{
			name: "JSON Array",
			message: &db.WebSocketMessage{
				PayloadData: `[1, 2, "three", {"name": "item"}]`,
			},
			scoped:         []string{"ws_json"},
			expectedCount:  12, // 1 array + 4 items + 4 indexes + 1 nested obj + 1 key + 1 value
			expectTypes:    []InsertionPointType{InsertionPointTypeWSJSONArray, InsertionPointTypeWSJSONArrayItem, InsertionPointTypeWSJSONArrayIndex},
			shouldNotError: true,
		},
		{
			name: "XML",
			message: &db.WebSocketMessage{
				PayloadData: `<user name="john" role="admin"><auth token="abc123"/></user>`,
			},
			scoped:         []string{"ws_xml"},
			expectedCount:  6, // 1 document + 2 tags (user, auth) + 3 attributes (name, role, token)
			expectTypes:    []InsertionPointType{InsertionPointTypeWSXMLElement, InsertionPointTypeWSXMLTag, InsertionPointTypeWSXMLAttribute},
			shouldNotError: true,
		},
		{
			name: "Multiple scopes",
			message: &db.WebSocketMessage{
				PayloadData: `{"message": "hello"}`,
			},
			scoped:         []string{"ws_raw", "ws_json"},
			expectedCount:  5, // 1 raw + 1 object + 1 key + 1 value + 1 field
			shouldNotError: true,
		},
		{
			name: "Invalid JSON",
			message: &db.WebSocketMessage{
				PayloadData: `{"broken": "json"`,
			},
			scoped:         []string{"ws_json"},
			expectedCount:  0,
			shouldNotError: true, // Should not error but return empty points
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			points, err := GetWebSocketMessageInsertionPoints(tc.message, tc.scoped)

			if tc.shouldNotError && err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if !tc.shouldNotError && err == nil {
				t.Fatalf("Expected error, got none")
			}

			if len(points) != tc.expectedCount {
				t.Errorf("Expected %d insertion points, got %d", tc.expectedCount, len(points))
			}

			// Check that we have the expected types
			if len(tc.expectTypes) > 0 {
				foundTypes := make(map[InsertionPointType]bool)
				for _, point := range points {
					foundTypes[point.Type] = true
				}

				for _, expectedType := range tc.expectTypes {
					if !foundTypes[expectedType] {
						t.Errorf("Expected to find insertion point type %s, but didn't", expectedType)
					}
				}
			}
		})
	}
}

func TestCreateModifiedWebSocketMessage(t *testing.T) {
	tests := []struct {
		name           string
		message        *db.WebSocketMessage
		insertionPoint InsertionPoint
		payload        string
		expectedResult string
		shouldNotError bool
	}{
		{
			name: "Modify raw message",
			message: &db.WebSocketMessage{
				PayloadData: "Hello, World!",
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSRawMessage,
				Name:      "message",
				Value:     "Hello, World!",
				ValueType: lib.TypeString,
			},
			payload:        "Modified message",
			expectedResult: "Modified message",
			shouldNotError: true,
		},
		{
			name: "Modify JSON field value",
			message: &db.WebSocketMessage{
				PayloadData: `{"user": "john", "role": "admin"}`,
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSJSONValue,
				Name:      "user",
				Value:     "john",
				ValueType: lib.TypeString,
			},
			payload:        "jane",
			expectedResult: `{"user":"jane","role":"admin"}`,
			shouldNotError: true,
		},
		{
			name: "Modify JSON key",
			message: &db.WebSocketMessage{
				PayloadData: `{"user": "john", "role": "admin"}`,
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSJSONKey,
				Name:      "user",
				Value:     "user",
				ValueType: lib.TypeString,
			},
			payload:        "username",
			expectedResult: `{"username":"john","role":"admin"}`,
			shouldNotError: true,
		},
		{
			name: "Modify JSON array item",
			message: &db.WebSocketMessage{
				PayloadData: `["one", "two", "three"]`,
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSJSONArrayItem,
				Name:      "root[1]",
				Value:     "two",
				ValueType: lib.TypeString,
			},
			payload:        "modified",
			expectedResult: `["one","modified","three"]`,
			shouldNotError: true,
		},
		{
			name: "Modify XML attribute",
			message: &db.WebSocketMessage{
				PayloadData: `<user name="john" role="admin" />`,
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSXMLAttribute,
				Name:      "name",
				Value:     "john",
				ValueType: lib.TypeString,
			},
			payload:        "jane",
			expectedResult: `<user name="jane" role="admin" />`,
			shouldNotError: true,
		},
		{
			name: "Modify XML tag",
			message: &db.WebSocketMessage{
				PayloadData: `<user>John</user>`,
			},
			insertionPoint: InsertionPoint{
				Type:      InsertionPointTypeWSXMLTag,
				Name:      "user",
				Value:     "user",
				ValueType: lib.TypeString,
			},
			payload:        "person",
			expectedResult: `<person>John</person>`,
			shouldNotError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modified, err := CreateModifiedWebSocketMessage(tc.message, tc.insertionPoint, tc.payload)

			if tc.shouldNotError && err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if !tc.shouldNotError && err == nil {
				t.Fatalf("Expected error, got none")
			}

			if modified == nil {
				if tc.shouldNotError {
					t.Fatalf("Expected modified message, got nil")
				}
				return
			}

			// For JSON, normalize the output for comparison
			if strings.HasPrefix(tc.expectedResult, "{") || strings.HasPrefix(tc.expectedResult, "[") {
				var expected, actual interface{}

				if err := json.Unmarshal([]byte(tc.expectedResult), &expected); err != nil {
					t.Fatalf("Failed to parse expected JSON: %v", err)
				}

				if err := json.Unmarshal([]byte(modified.PayloadData), &actual); err != nil {
					t.Fatalf("Failed to parse actual JSON: %v", err)
				}

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			} else {
				// For non-JSON, compare strings directly
				if tc.expectedResult != modified.PayloadData {
					t.Errorf("Expected %s, got %s", tc.expectedResult, modified.PayloadData)
				}
			}
		})
	}
}

// Test helper functions
func TestParseArrayAccess(t *testing.T) {
	tests := []struct {
		path          string
		expectedIdx   int
		expectedName  string
		expectedFound bool
	}{
		{"users[0]", 0, "users", true},
		{"data[42]", 42, "data", true},
		{"root.items[5]", 5, "root.items", true},
		{"users", 0, "", false},
		{"users[]", 0, "", false},
		{"users[abc]", 0, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			idx, name, found := parseArrayAccess(tc.path)

			if found != tc.expectedFound {
				t.Errorf("Expected found=%v, got %v", tc.expectedFound, found)
			}

			if !found {
				return
			}

			if idx != tc.expectedIdx {
				t.Errorf("Expected idx=%d, got %d", tc.expectedIdx, idx)
			}

			if name != tc.expectedName {
				t.Errorf("Expected name=%s, got %s", tc.expectedName, name)
			}
		})
	}
}

func TestIsLikelyJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`{"name": "John"}`, true},
		{`[1, 2, 3]`, true},
		{`  {"name": "John"}  `, true},
		{`  [1, 2, 3]  `, true},
		{`{"name": "John", "age": 30}`, true},
		{`[{"name": "John"}, {"name": "Jane"}]`, true},
		{`{"name": "John", "friends": ["Jane", "Doe"]}`, true},
		{`{"name": "John", "address": {"city": "New York"}}`, true},
		{`{"name": "John", "age": 30, "isActive": true}`, true},
		{`<xml>not json</xml>`, false},
		{`plain text`, false},
		{`{broken json`, false},
		{`[broken json`, false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := isLikelyJSON(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for input %s", tc.expected, result, tc.input)
			}
		})
	}
}

func TestIsLikelyXML(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`<user>John</user>`, true},
		{`<user name="John" />`, true},
		{`  <user>John</user>  `, true},
		{`  <user name="John" />  `, true},
		{`<user xmlns="http://example.com">John</user>`, true},
		{`{"name": "John"}`, false},
		{`plain text`, false},
		{`<broken xml`, false},
		{`<user>John</user><user>Jane</user>`, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := isLikelyXML(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for input %s", tc.expected, result, tc.input)
			}
		})
	}
}
