package openapi

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Parse parses the OpenAPI content and returns a Document
func Parse(content []byte) (*Document, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromData(content)
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	// if err := doc.Validate(loader.Context); err != nil {
	// 	return nil, fmt.Errorf("failed to validate openapi spec: %w", err)
	// }

	return &Document{spec: doc}, nil
}
