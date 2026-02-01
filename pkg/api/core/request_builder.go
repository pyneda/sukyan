package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type RequestBuilder interface {
	Build(ctx context.Context, op Operation, paramValues map[string]any) (*http.Request, error)
	BuildWithModifiedParam(ctx context.Context, op Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error)
	GetDefaultParamValues(op Operation) map[string]any
}

type BaseRequestBuilder struct {
	DefaultHeaders map[string]string
}

func NewBaseRequestBuilder() *BaseRequestBuilder {
	return &BaseRequestBuilder{
		DefaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
		},
	}
}

func (b *BaseRequestBuilder) GetDefaultParamValues(op Operation) map[string]any {
	values := make(map[string]any)
	for _, param := range op.Parameters {
		values[param.Name] = param.GetEffectiveValue()
	}
	return values
}

func (b *BaseRequestBuilder) Build(ctx context.Context, op Operation, paramValues map[string]any) (*http.Request, error) {
	fullURL, err := b.buildURL(op, paramValues)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	var body []byte
	var contentType string

	if op.HasBodyParameters() {
		body, contentType, err = b.buildBody(op, paramValues)
		if err != nil {
			return nil, fmt.Errorf("building body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	b.addHeaderParams(req, op, paramValues)
	b.addCookieParams(req, op, paramValues)
	b.addDefaultHeaders(req)

	return req, nil
}

func (b *BaseRequestBuilder) BuildWithModifiedParam(ctx context.Context, op Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error) {
	modifiedValues := make(map[string]any)
	for k, v := range paramValues {
		modifiedValues[k] = v
	}
	modifiedValues[paramName] = modifiedValue
	return b.Build(ctx, op, modifiedValues)
}

func (b *BaseRequestBuilder) buildURL(op Operation, paramValues map[string]any) (string, error) {
	baseURL := strings.TrimSuffix(op.BaseURL, "/")
	path := op.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	for _, param := range op.Parameters {
		if param.Location == ParameterLocationPath {
			value := paramValues[param.Name]
			if value == nil {
				value = param.GetEffectiveValue()
			}
			placeholder := "{" + param.Name + "}"
			path = strings.ReplaceAll(path, placeholder, fmt.Sprintf("%v", value))
		}
	}

	fullURL := baseURL + path

	queryParams := url.Values{}
	for _, param := range op.Parameters {
		if param.Location == ParameterLocationQuery {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}
			queryParams.Set(param.Name, fmt.Sprintf("%v", value))
		}
	}

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	return fullURL, nil
}

func (b *BaseRequestBuilder) buildBody(op Operation, paramValues map[string]any) ([]byte, string, error) {
	bodyParams := make(map[string]any)
	for _, param := range op.Parameters {
		if param.Location == ParameterLocationBody {
			value := paramValues[param.Name]
			if value == nil {
				value = param.GetEffectiveValue()
			}
			bodyParams[param.Name] = value
		}
	}

	if len(bodyParams) == 0 {
		return nil, "", nil
	}

	body, err := json.Marshal(bodyParams)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling body: %w", err)
	}

	return body, "application/json", nil
}

func (b *BaseRequestBuilder) addHeaderParams(req *http.Request, op Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == ParameterLocationHeader {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}
			req.Header.Set(param.Name, fmt.Sprintf("%v", value))
		}
	}
}

func (b *BaseRequestBuilder) addCookieParams(req *http.Request, op Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == ParameterLocationCookie {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}
			req.AddCookie(&http.Cookie{
				Name:  param.Name,
				Value: fmt.Sprintf("%v", value),
			})
		}
	}
}

func (b *BaseRequestBuilder) addDefaultHeaders(req *http.Request) {
	for k, v := range b.DefaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

type RequestBuilderOptions struct {
	AuthToken       string
	AuthTokenPrefix string
	APIKey          string
	APIKeyHeader    string
	CustomHeaders   map[string]string
}

func (b *BaseRequestBuilder) ApplyAuth(req *http.Request, opts RequestBuilderOptions) {
	if opts.AuthToken != "" {
		prefix := opts.AuthTokenPrefix
		if prefix == "" {
			prefix = "Bearer"
		}
		req.Header.Set("Authorization", prefix+" "+opts.AuthToken)
	}

	if opts.APIKey != "" && opts.APIKeyHeader != "" {
		req.Header.Set(opts.APIKeyHeader, opts.APIKey)
	}

	for k, v := range opts.CustomHeaders {
		req.Header.Set(k, v)
	}
}
