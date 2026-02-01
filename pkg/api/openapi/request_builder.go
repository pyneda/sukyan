package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/pkg/api/core"
)

type RequestBuilder struct {
	DefaultHeaders map[string]string
	AuthConfig     *AuthConfig
}

type AuthConfig struct {
	BearerToken   string
	BasicUsername string
	BasicPassword string
	APIKey        string
	APIKeyHeader  string
	APIKeyIn      string
	CustomHeaders map[string]string
}

func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		DefaultHeaders: map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
			"Accept":     "application/json, */*",
		},
	}
}

func (b *RequestBuilder) WithAuth(config *AuthConfig) *RequestBuilder {
	b.AuthConfig = config
	return b
}

func (b *RequestBuilder) Build(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	fullURL, err := b.buildURL(op, paramValues)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}

	var body []byte
	contentType := "application/json"

	if op.HasBodyParameters() {
		body, contentType, err = b.buildBody(op, paramValues)
		if err != nil {
			return nil, fmt.Errorf("building body: %w", err)
		}
	}

	method := op.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if len(body) > 0 && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	b.addHeaderParams(req, op, paramValues)
	b.addCookieParams(req, op, paramValues)
	b.addDefaultHeaders(req)
	b.applyAuth(req)

	return req, nil
}

func (b *RequestBuilder) BuildWithModifiedParam(ctx context.Context, op core.Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error) {
	modifiedValues := make(map[string]any)
	for k, v := range paramValues {
		modifiedValues[k] = v
	}
	modifiedValues[paramName] = modifiedValue
	return b.Build(ctx, op, modifiedValues)
}

func (b *RequestBuilder) GetDefaultParamValues(op core.Operation) map[string]any {
	values := make(map[string]any)
	for _, param := range op.Parameters {
		values[param.Name] = param.GetEffectiveValue()
	}
	return values
}

func (b *RequestBuilder) buildURL(op core.Operation, paramValues map[string]any) (string, error) {
	baseURL := strings.TrimSuffix(op.BaseURL, "/")
	path := op.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationPath {
			value := paramValues[param.Name]
			if value == nil {
				value = param.GetEffectiveValue()
			}
			placeholder := "{" + param.Name + "}"
			encoded := url.PathEscape(fmt.Sprintf("%v", value))
			path = strings.ReplaceAll(path, placeholder, encoded)
		}
	}

	fullURL := baseURL + path

	queryParams := url.Values{}
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationQuery {
			value := paramValues[param.Name]
			if value == nil && !param.Required {
				continue
			}
			if value == nil {
				value = param.GetEffectiveValue()
			}

			switch v := value.(type) {
			case []any:
				for _, item := range v {
					queryParams.Add(param.Name, fmt.Sprintf("%v", item))
				}
			case []string:
				for _, item := range v {
					queryParams.Add(param.Name, item)
				}
			default:
				queryParams.Set(param.Name, fmt.Sprintf("%v", value))
			}
		}
	}

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	return fullURL, nil
}

func (b *RequestBuilder) buildBody(op core.Operation, paramValues map[string]any) ([]byte, string, error) {
	bodyParams := make(map[string]any)

	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationBody {
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

	contentType := "application/json"
	if op.OpenAPI != nil && op.OpenAPI.RequestBody != nil && op.OpenAPI.RequestBody.ContentType != "" {
		contentType = op.OpenAPI.RequestBody.ContentType
	}

	var body []byte
	var err error

	switch contentType {
	case "application/x-www-form-urlencoded":
		formValues := url.Values{}
		for k, v := range bodyParams {
			formValues.Set(k, fmt.Sprintf("%v", v))
		}
		body = []byte(formValues.Encode())
	case "multipart/form-data":
		buf := new(bytes.Buffer)
		writer := multipart.NewWriter(buf)
		for k, v := range bodyParams {
			if err := writer.WriteField(k, fmt.Sprintf("%v", v)); err != nil {
				return nil, "", fmt.Errorf("writing multipart field %s: %w", k, err)
			}
		}
		if err := writer.Close(); err != nil {
			return nil, "", fmt.Errorf("closing multipart writer: %w", err)
		}
		body = buf.Bytes()
		contentType = writer.FormDataContentType()
	default:
		body, err = json.Marshal(bodyParams)
		if err != nil {
			return nil, "", fmt.Errorf("marshaling body: %w", err)
		}
	}

	return body, contentType, nil
}

func (b *RequestBuilder) addHeaderParams(req *http.Request, op core.Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationHeader {
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

func (b *RequestBuilder) addCookieParams(req *http.Request, op core.Operation, paramValues map[string]any) {
	for _, param := range op.Parameters {
		if param.Location == core.ParameterLocationCookie {
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

func (b *RequestBuilder) addDefaultHeaders(req *http.Request) {
	for k, v := range b.DefaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func (b *RequestBuilder) applyAuth(req *http.Request) {
	if b.AuthConfig == nil {
		return
	}

	if b.AuthConfig.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.AuthConfig.BearerToken)
	}

	if b.AuthConfig.BasicUsername != "" || b.AuthConfig.BasicPassword != "" {
		req.SetBasicAuth(b.AuthConfig.BasicUsername, b.AuthConfig.BasicPassword)
	}

	if b.AuthConfig.APIKey != "" && b.AuthConfig.APIKeyHeader != "" {
		switch b.AuthConfig.APIKeyIn {
		case "query":
			q := req.URL.Query()
			q.Set(b.AuthConfig.APIKeyHeader, b.AuthConfig.APIKey)
			req.URL.RawQuery = q.Encode()
		case "cookie":
			req.AddCookie(&http.Cookie{
				Name:  b.AuthConfig.APIKeyHeader,
				Value: b.AuthConfig.APIKey,
			})
		default:
			req.Header.Set(b.AuthConfig.APIKeyHeader, b.AuthConfig.APIKey)
		}
	}

	for k, v := range b.AuthConfig.CustomHeaders {
		req.Header.Set(k, v)
	}
}

func BuildRequest(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	builder := NewRequestBuilder()
	return builder.Build(ctx, op, paramValues)
}
