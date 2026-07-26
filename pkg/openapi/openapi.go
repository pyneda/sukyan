package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultMaxSpecSize bounds how much document data is parsed. Specs are fetched
	// from scan targets, so an unbounded read is a denial of service vector.
	DefaultMaxSpecSize = 32 << 20
	remoteRefTimeout      = 10 * time.Second
	maxRemoteRefSize      = 8 << 20
	maxRemoteRefRedirects = 5
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// ParseOptions controls how a document is loaded.
type ParseOptions struct {
	// SourceURL is where the document was retrieved from. It resolves relative
	// server URLs and constrains which origin external references may come from.
	SourceURL string

	// AllowRemoteRefs permits resolving external $refs over HTTP. References are
	// restricted to the origin of SourceURL; local filesystem references are never
	// followed. Without it, external references are pruned instead of fetched.
	AllowRemoteRefs bool

	// MaxSize overrides DefaultMaxSpecSize.
	MaxSize int
}

// Parse loads an OpenAPI or Swagger document without touching the network or
// filesystem. Use ParseWithOptions to resolve external references.
func Parse(content []byte) (*Document, error) {
	return ParseWithOptions(content, ParseOptions{})
}

// ParseWithOptions loads an OpenAPI 3.x or Swagger 2.0 document, in JSON or YAML.
// Swagger 2.0 documents are converted to OpenAPI 3 so that host, basePath, schemes,
// body/formData parameters and securityDefinitions survive.
func ParseWithOptions(content []byte, opts ParseOptions) (*Document, error) {
	maxSize := opts.MaxSize
	if maxSize <= 0 {
		maxSize = DefaultMaxSpecSize
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New("empty openapi document")
	}
	if len(content) > maxSize {
		return nil, fmt.Errorf("openapi document is %d bytes, exceeds limit of %d", len(content), maxSize)
	}

	data, err := toJSON(content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode openapi document: %w", err)
	}

	version := specVersion(data)
	if version.OpenAPI == "" && version.Swagger == "" {
		return nil, errors.New("not an openapi document: missing openapi or swagger version field")
	}

	var spec *openapi3.T
	if version.isSwagger2() {
		spec, err = convertSwagger2(data, opts)
	} else {
		spec, err = loadV3(normalizeVersion31(data, version), opts)
	}
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, errors.New("failed to load openapi spec: empty document")
	}

	return &Document{spec: spec, sourceURL: opts.SourceURL}, nil
}

// toJSON normalises the document to JSON, accepting YAML input and tolerating a
// leading BOM or whitespace.
func toJSON(content []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(content, utf8BOM))
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return trimmed, nil
	}

	var node interface{}
	if err := yaml.Unmarshal(trimmed, &node); err != nil {
		return nil, err
	}
	converted, err := json.Marshal(normalizeYAML(node))
	if err != nil {
		return nil, err
	}
	return converted, nil
}

// normalizeYAML rewrites the map[interface{}]interface{} values YAML can produce for
// non-string keys into JSON-encodable maps.
func normalizeYAML(node interface{}) interface{} {
	switch typed := node.(type) {
	case map[string]interface{}:
		for k, v := range typed {
			typed[k] = normalizeYAML(v)
		}
		return typed
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			converted[fmt.Sprintf("%v", k)] = normalizeYAML(v)
		}
		return converted
	case []interface{}:
		for i, v := range typed {
			typed[i] = normalizeYAML(v)
		}
		return typed
	default:
		return node
	}
}

type specVersionInfo struct {
	Swagger string `json:"swagger"`
	OpenAPI string `json:"openapi"`
}

func (v specVersionInfo) isSwagger2() bool {
	return v.OpenAPI == "" && strings.HasPrefix(v.Swagger, "2.")
}

func (v specVersionInfo) is31() bool {
	return strings.HasPrefix(v.OpenAPI, "3.1")
}

func specVersion(data []byte) specVersionInfo {
	var probe specVersionInfo
	if err := json.Unmarshal(data, &probe); err != nil {
		return specVersionInfo{}
	}
	return probe
}

// normalizeVersion31 rewrites the OpenAPI 3.1 keywords that kin-openapi still models
// the 3.0 way. Without this a 3.1 document using numeric exclusiveMinimum fails to
// unmarshal and the entire definition is discarded.
func normalizeVersion31(data []byte, version specVersionInfo) []byte {
	if !version.is31() {
		return data
	}
	var node interface{}
	if err := json.Unmarshal(data, &node); err != nil {
		return data
	}
	converted, err := json.Marshal(downgrade31(node, false))
	if err != nil {
		return data
	}
	return converted
}

var (
	// schemaMapKeys hold maps whose keys are author-chosen names and whose values are
	// schemas. Their contents must be recursed into, but the map itself is not a
	// schema: a property literally named "const" is not the const keyword.
	schemaMapKeys = map[string]bool{
		"properties": true, "patternProperties": true, "definitions": true,
		"schemas": true, "$defs": true,
	}
	// schemaValueKeys hold a single nested schema.
	schemaValueKeys = map[string]bool{
		"schema": true, "items": true, "additionalProperties": true, "not": true,
		"if": true, "then": true, "else": true, "contains": true, "propertyNames": true,
	}
	// schemaListKeys hold arrays of schemas.
	schemaListKeys = map[string]bool{
		"allOf": true, "anyOf": true, "oneOf": true, "prefixItems": true,
	}
	// dataKeys hold arbitrary author data that must never be rewritten.
	dataKeys = map[string]bool{
		"example": true, "examples": true, "default": true, "enum": true, "const": true,
	}
)

// downgrade31 rewrites the OpenAPI 3.1 keywords kin-openapi still models the 3.0 way.
// isSchema tracks whether the current object is a schema, so that a name-keyed map
// such as "properties" is traversed without treating its keys as keywords.
func downgrade31(node interface{}, isSchema bool) interface{} {
	switch typed := node.(type) {
	case map[string]interface{}:
		if isSchema {
			for _, bound := range []struct{ exclusive, inclusive string }{
				{"exclusiveMinimum", "minimum"},
				{"exclusiveMaximum", "maximum"},
			} {
				if value, ok := typed[bound.exclusive]; ok {
					if _, isBool := value.(bool); !isBool {
						typed[bound.inclusive] = value
						typed[bound.exclusive] = true
					}
				}
			}
			if value, ok := typed["const"]; ok {
				if _, hasEnum := typed["enum"]; !hasEnum {
					typed["enum"] = []interface{}{value}
				}
			}
		}

		for key, value := range typed {
			switch {
			case isSchema && dataKeys[key]:
				// Author data, not schema structure.
			case schemaMapKeys[key]:
				if entries, ok := value.(map[string]interface{}); ok {
					for name, entry := range entries {
						entries[name] = downgrade31(entry, true)
					}
				}
			case schemaValueKeys[key]:
				typed[key] = downgrade31(value, true)
			case schemaListKeys[key]:
				if entries, ok := value.([]interface{}); ok {
					for i, entry := range entries {
						entries[i] = downgrade31(entry, true)
					}
				}
			default:
				typed[key] = downgrade31(value, false)
			}
		}
		return typed
	case []interface{}:
		for i, value := range typed {
			typed[i] = downgrade31(value, false)
		}
		return typed
	default:
		return node
	}
}

func convertSwagger2(data []byte, opts ParseOptions) (*openapi3.T, error) {
	var doc2 openapi2.T
	if err := json.Unmarshal(data, &doc2); err != nil {
		log.Debug().Err(err).Msg("Failed to decode swagger 2.0 document, falling back to openapi 3 loader")
		return loadV3(data, opts)
	}

	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil || doc3 == nil {
		log.Debug().Err(err).Msg("Failed to convert swagger 2.0 document, falling back to openapi 3 loader")
		return loadV3(data, opts)
	}

	if err := newLoader(opts).ResolveRefsIn(doc3, refLocation(opts)); err != nil {
		log.Debug().Err(err).Msg("Failed to resolve references in converted swagger 2.0 document")
	}
	return doc3, nil
}

func loadV3(data []byte, opts ParseOptions) (*openapi3.T, error) {
	spec, err := load(data, opts)
	if err == nil {
		return spec, nil
	}

	// A document whose external references cannot be resolved is still worth
	// scanning: keep every path and operation rather than discarding the spec.
	pruned, changed := stripExternalRefs(data)
	if !changed {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}
	spec, prunedErr := load(pruned, ParseOptions{SourceURL: opts.SourceURL})
	if prunedErr != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}
	log.Debug().Err(err).Msg("Loaded openapi spec after dropping unresolvable external references")
	return spec, nil
}

func load(data []byte, opts ParseOptions) (*openapi3.T, error) {
	loader := newLoader(opts)
	if location := refLocation(opts); location != nil {
		return loader.LoadFromDataWithPath(data, location)
	}
	return loader.LoadFromData(data)
}

func refLocation(opts ParseOptions) *url.URL {
	if opts.SourceURL == "" {
		return nil
	}
	location, err := url.Parse(opts.SourceURL)
	if err != nil || !location.IsAbs() {
		return nil
	}
	return location
}

// newLoader builds a loader that never reads local files and, when remote
// references are enabled, only fetches them from the origin the spec came from.
// The kin-openapi default reader resolves $refs through http.DefaultClient and the
// local filesystem, which turns parsing a target-supplied spec into SSRF and local
// file disclosure.
func newLoader(opts ParseOptions) *openapi3.Loader {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	if !opts.AllowRemoteRefs {
		return loader
	}
	origin := refLocation(opts)
	if origin == nil || (origin.Scheme != "http" && origin.Scheme != "https") {
		return loader
	}

	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = sameOriginReader(origin)
	return loader
}

func sameOriginReader(origin *url.URL) openapi3.ReadFromURIFunc {
	// Redirects are checked too: validating only the initial URL lets a target answer
	// a same-origin reference with a 302 to an internal address, which would put the
	// SSRF straight back.
	client := &http.Client{
		Timeout: remoteRefTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRemoteRefRedirects {
				return fmt.Errorf("too many redirects resolving reference %q", req.URL.String())
			}
			return checkSameOrigin(req.URL, origin)
		},
	}
	return func(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
		if location == nil {
			return nil, errors.New("missing reference location")
		}
		if err := checkSameOrigin(location, origin); err != nil {
			return nil, err
		}

		resp, err := client.Get(location.String())
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("reference %q returned status %d", location.String(), resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, maxRemoteRefSize))
	}
}

func checkSameOrigin(location, origin *url.URL) error {
	if location.Scheme != "http" && location.Scheme != "https" {
		return fmt.Errorf("refusing to read non-http reference %q", location.String())
	}
	if !strings.EqualFold(location.Host, origin.Host) {
		return fmt.Errorf("refusing cross-origin reference %q", location.String())
	}
	return nil
}

// stripExternalRefs replaces every non-local $ref with an empty schema so the rest
// of the document can still be loaded.
func stripExternalRefs(data []byte) ([]byte, bool) {
	var node interface{}
	if err := json.Unmarshal(data, &node); err != nil {
		return data, false
	}

	pruned, changed := pruneExternalRefs(node)
	if !changed {
		return data, false
	}
	encoded, err := json.Marshal(pruned)
	if err != nil {
		return data, false
	}
	return encoded, true
}

func pruneExternalRefs(node interface{}) (interface{}, bool) {
	switch typed := node.(type) {
	case map[string]interface{}:
		if ref, ok := typed["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
			return map[string]interface{}{}, true
		}
		changed := false
		for k, v := range typed {
			replacement, didChange := pruneExternalRefs(v)
			if didChange {
				typed[k] = replacement
				changed = true
			}
		}
		return typed, changed
	case []interface{}:
		changed := false
		for i, v := range typed {
			replacement, didChange := pruneExternalRefs(v)
			if didChange {
				typed[i] = replacement
				changed = true
			}
		}
		return typed, changed
	default:
		return node, false
	}
}
