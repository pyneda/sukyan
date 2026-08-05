package openapifixtures

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed content_type_cases.json
var contentTypeCasesJSON []byte

// ContentTypePath is the path every ContentTypeCase spec puts its operation at.
const ContentTypePath = "/submit"

// ContentTypeFields are the named properties a structured media type in a case's spec
// carries. Selecting an opaque media type instead collapses them into a single unnamed
// body parameter, which is how a form login loses both of its insertion points.
var ContentTypeFields = []string{"password", "username"}

// ContentTypeCase is one request body offered under several media types. Which one to
// send is a choice, and both parsers have to make it identically: the scanner selecting
// a different media type than the generator means the request the playground and the
// report describe is not the request that gets tested.
type ContentTypeCase struct {
	Name   string   `json:"name"`
	Offers []string `json:"offers"`
	Want   string   `json:"want"`
}

// ContentTypeCases returns every offered-set both parsers have to resolve alike.
func ContentTypeCases() ([]ContentTypeCase, error) {
	var cases []ContentTypeCase
	if err := json.Unmarshal(contentTypeCasesJSON, &cases); err != nil {
		return nil, fmt.Errorf("parsing content_type_cases.json: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("content_type_cases.json declares no cases")
	}
	for _, c := range cases {
		if len(c.Offers) == 0 || c.Want == "" {
			return nil, fmt.Errorf("case %q declares no offers or no expected selection", c.Name)
		}
	}
	return cases, nil
}

// OpaqueMediaType reports whether a media type carries bytes rather than named fields.
// A spec spells such a body as a binary string, so no part of it can be addressed and a
// parser that selects one has nothing left to fuzz.
func OpaqueMediaType(mediaType string) bool {
	switch mediaType {
	case "application/octet-stream", "application/pdf":
		return true
	}
	return strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/")
}

// Spec builds one operation whose request body is offered under every media type the
// case names, each carrying the schema that media type would realistically be given.
func (c ContentTypeCase) Spec() (string, error) {
	properties := make(map[string]any, len(ContentTypeFields))
	for _, field := range ContentTypeFields {
		properties[field] = map[string]any{"type": "string"}
	}
	structured := map[string]any{"type": "object", "required": ContentTypeFields, "properties": properties}
	opaque := map[string]any{"type": "string", "format": "binary"}

	content := make(map[string]any, len(c.Offers))
	for _, mediaType := range c.Offers {
		schema := structured
		if OpaqueMediaType(mediaType) {
			schema = opaque
		}
		content[mediaType] = map[string]any{"schema": schema}
	}

	spec := map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": "Content types", "version": "1.0.0"},
		"servers": []any{map[string]any{"url": "http://api.test"}},
		"paths": map[string]any{ContentTypePath: map[string]any{"post": map[string]any{
			"operationId": "submit",
			"requestBody": map[string]any{"content": content},
			"responses":   map[string]any{"200": map[string]any{"description": "ok"}},
		}}},
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
