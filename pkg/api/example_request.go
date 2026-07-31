package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

const maskPlaceholder = "****"

// ExampleRequest is one concrete request for an operation, rendered as raw HTTP.
type ExampleRequest struct {
	Raw    string   `json:"raw"`
	URL    string   `json:"url"`
	Method string   `json:"method"`
	Masked []string `json:"masked,omitempty"`
}

// BuildExampleRequest renders the request an operation's default parameter values
// produce, with the definition's auth applied.
//
// Credentials are replaced with a placeholder unless reveal is set: this is a
// reference panel that gets screenshared, and the header names alone are what tell
// the reader which schemes apply.
func BuildExampleRequest(
	ctx context.Context,
	defType db.APIDefinitionType,
	op *core.Operation,
	cfg *db.APIAuthConfig,
	reveal bool,
) (*ExampleRequest, error) {
	if op == nil {
		return nil, fmt.Errorf("no operation to build a request from")
	}

	// BuildDefaultRequest's variadic last parameter is a parsed GraphQL schema, used
	// to expand selection sets. It is deliberately not passed: obtaining one means
	// parsing the definition, which is the cost this design removes from the read
	// path. GraphQL examples carry the operation and its arguments without a fully
	// expanded selection set.
	req, err := BuildDefaultRequest(ctx, defType, op)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	masked := []string{}
	if cfg != nil {
		ApplyAuthConfig(req, cfg, "")
		if !reveal {
			masked = maskCredentials(req, cfg)
		}
	}
	sort.Strings(masked)

	raw, err := dumpRequest(req)
	if err != nil {
		return nil, fmt.Errorf("serializing request: %w", err)
	}

	return &ExampleRequest{
		Raw:    raw,
		URL:    req.URL.String(),
		Method: req.Method,
		Masked: masked,
	}, nil
}

// maskCredentials overwrites the values ApplyAuthConfig just wrote and reports which
// names it touched. It works from the config rather than scanning for secret-looking
// strings, so it cannot miss one or mangle an unrelated header.
func maskCredentials(req *http.Request, cfg *db.APIAuthConfig) []string {
	masked := []string{}

	switch cfg.Type {
	case db.APIAuthTypeBasic:
		if req.Header.Get("Authorization") != "" {
			req.Header.Set("Authorization", "Basic "+maskPlaceholder)
			masked = append(masked, "Authorization")
		}

	case db.APIAuthTypeBearer, db.APIAuthTypeOAuth2:
		if req.Header.Get("Authorization") != "" {
			req.Header.Set("Authorization", bearerPrefix(cfg)+" "+maskPlaceholder)
			masked = append(masked, "Authorization")
		}

	case db.APIAuthTypeAPIKey:
		if cfg.APIKeyName == "" {
			break
		}
		switch cfg.APIKeyLocation {
		case db.APIKeyLocationHeader:
			if req.Header.Get(cfg.APIKeyName) != "" {
				req.Header.Set(cfg.APIKeyName, maskPlaceholder)
				masked = append(masked, cfg.APIKeyName)
			}
		case db.APIKeyLocationQuery:
			query := req.URL.Query()
			if query.Get(cfg.APIKeyName) != "" {
				query.Set(cfg.APIKeyName, maskPlaceholder)
				req.URL.RawQuery = query.Encode()
				masked = append(masked, cfg.APIKeyName)
			}
		case db.APIKeyLocationCookie:
			if cookie, err := req.Cookie(cfg.APIKeyName); err == nil && cookie.Value != "" {
				req.Header.Del("Cookie")
				req.AddCookie(&http.Cookie{Name: cfg.APIKeyName, Value: maskPlaceholder})
				masked = append(masked, cfg.APIKeyName)
			}
		}
	}

	for _, header := range cfg.CustomHeaders {
		if req.Header.Get(header.HeaderName) != "" {
			req.Header.Set(header.HeaderName, maskPlaceholder)
			masked = append(masked, header.HeaderName)
		}
	}

	return masked
}

// dumpRequest renders a request as raw HTTP for a human to read.
//
// httputil.DumpRequestOut is not used because it rewrites the request for the wire —
// adding Accept-Encoding and a transfer encoding the specification never declared —
// and this output is read, not sent. Headers are sorted so two reads of the same
// operation produce the same text.
func dumpRequest(req *http.Request) (string, error) {
	var body []byte
	if req.Body != nil {
		read, err := io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}
		req.Body = io.NopCloser(bytes.NewReader(read))
		body = read
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())
	fmt.Fprintf(&out, "Host: %s\r\n", req.URL.Host)

	names := make([]string, 0, len(req.Header))
	for name := range req.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range req.Header[name] {
			fmt.Fprintf(&out, "%s: %s\r\n", name, value)
		}
	}

	out.WriteString("\r\n")
	out.Write(body)
	return out.String(), nil
}
