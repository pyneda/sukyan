package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/internal/openapifixtures"
)

func compositionSpecs(t *testing.T) map[string]string {
	t.Helper()
	shapes, err := openapifixtures.CompositionShapes()
	if err != nil {
		t.Fatal(err)
	}
	specs := make(map[string]string, len(shapes))
	for _, shape := range shapes {
		spec, err := shape.Spec()
		if err != nil {
			t.Fatalf("building the %s spec: %v", shape.Name, err)
		}
		specs[shape.Name] = spec
	}
	return specs
}

func happyPathBody(t *testing.T, endpoints []Endpoint, path string) any {
	t.Helper()
	for _, endpoint := range endpoints {
		if !strings.HasSuffix(endpoint.Path, path) {
			continue
		}
		for _, request := range endpoint.Requests {
			if request.Label != "Happy Path" {
				continue
			}
			if len(request.Body) == 0 {
				return nil
			}
			var decoded any
			if err := json.Unmarshal(request.Body, &decoded); err != nil {
				t.Fatalf("%s body %q is not JSON: %v", path, request.Body, err)
			}
			return decoded
		}
	}
	t.Fatalf("no happy path request for %s", path)
	return nil
}

func wrappedValue(t *testing.T, body any, path string) any {
	t.Helper()
	object, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("%s body is %T, want an object wrapping the schema under test", path, body)
	}
	value, ok := object["value"]
	if !ok {
		t.Fatalf("%s body %+v has no value field", path, object)
	}
	return value
}

func encode(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func happyPathRequest(t *testing.T, endpoints []Endpoint, path string) RequestVariation {
	t.Helper()
	for _, endpoint := range endpoints {
		if !strings.HasSuffix(endpoint.Path, path) {
			continue
		}
		for _, request := range endpoint.Requests {
			if request.Label == "Happy Path" {
				return request
			}
		}
	}
	t.Fatalf("no happy path request for %s", path)
	return RequestVariation{}
}

// TestContentTypeSelectionAgreesWithTheScanner holds this package to the same media
// type pkg/api/openapi selects from the same body, against one shared list. A body
// offered as both a form and an opaque blob is the case that matters: whichever parser
// picks the blob is left with a single unnamed parameter where the other has named
// fields, so the report describes a request the scanner never sends.
func TestContentTypeSelectionAgreesWithTheScanner(t *testing.T) {
	cases, err := openapifixtures.ContentTypeCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			spec, err := testCase.Spec()
			if err != nil {
				t.Fatalf("building the %s spec: %v", testCase.Name, err)
			}
			request := happyPathRequest(t, generateHappyPath(t, spec), openapifixtures.ContentTypePath)

			// Multipart carries its boundary in the header, so only the media type
			// itself is comparable.
			mediaType, _, _ := strings.Cut(request.Headers["Content-Type"], ";")
			if strings.TrimSpace(mediaType) != testCase.Want {
				t.Fatalf("Content-Type = %q, want %q", request.Headers["Content-Type"], testCase.Want)
			}

			if openapifixtures.OpaqueMediaType(testCase.Want) {
				return
			}
			for _, field := range openapifixtures.ContentTypeFields {
				if !strings.Contains(string(request.Body), field) {
					t.Errorf("%s body %q does not carry the %s field", testCase.Want, request.Body, field)
				}
			}
		})
	}
}

// TestCompositionResolvesTheSameAtEveryDepth is the invariant the resolver exists for:
// what a schema describes cannot depend on where it sits. This package feeds the
// playground, the report and the CLI, so a disagreement with the scan-time parser
// means the UI describes a request the scanner never sends.
func TestCompositionResolvesTheSameAtEveryDepth(t *testing.T) {
	for name, spec := range compositionSpecs(t) {
		t.Run(name, func(t *testing.T) {
			endpoints := generateHappyPath(t, spec)

			root := happyPathBody(t, endpoints, "/root")
			nested := wrappedValue(t, happyPathBody(t, endpoints, "/nested"), "/nested")
			array := wrappedValue(t, happyPathBody(t, endpoints, "/array"), "/array")

			if root == nil {
				t.Fatal("/root sent no body — the schema resolved to nothing")
			}
			if encode(t, root) != encode(t, nested) {
				t.Errorf("nested value = %s, root = %s", encode(t, nested), encode(t, root))
			}

			items, ok := array.([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("/array value = %s, want a one-element array", encode(t, array))
			}
			if encode(t, root) != encode(t, items[0]) {
				t.Errorf("array item = %s, root = %s", encode(t, items[0]), encode(t, root))
			}
		})
	}
}

// A resolved value has to be something the endpoint can accept. Null leaves are what a
// cut composition used to serialise: the field is present, typed as nothing, and the
// API rejects the whole request before the handler is reached.
func TestCompositionEmitsNoNullLeaves(t *testing.T) {
	for name, spec := range compositionSpecs(t) {
		t.Run(name, func(t *testing.T) {
			endpoints := generateHappyPath(t, spec)
			for _, path := range []string{"/root", "/nested", "/array"} {
				body := encode(t, happyPathBody(t, endpoints, path))
				if strings.Contains(body, "null") {
					t.Errorf("%s body %s carries a null leaf", path, body)
				}
			}
		})
	}
}
