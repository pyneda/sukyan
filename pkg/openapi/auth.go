package openapi

import (
	"encoding/base64"
)

// AuthType defines the type of authentication
type AuthType string

const (
	AuthTypeBasic  AuthType = "basic"
	AuthTypeBearer AuthType = "bearer"
	AuthTypeAPIKey AuthType = "apikey"
)

// AuthConfig holds authentication details
type AuthConfig struct {
	Type  AuthType
	Key   string // User for Basic, Token for Bearer/APIKey
	Value string // Password for Basic
	In    string // header, query, cookie (for APIKey)
	Name  string // Header/Param name (for APIKey)
}

// ApplyToHeaders applies authentication to the provided headers map
func (a *AuthConfig) ApplyToHeaders(headers map[string]string) {
	if headers == nil {
		return
	}
	switch a.Type {
	case AuthTypeBasic:
		auth := a.Key + ":" + a.Value
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	case AuthTypeBearer:
		headers["Authorization"] = "Bearer " + a.Key
	case AuthTypeAPIKey:
		if a.In == "header" {
			headers[a.Name] = a.Key
		}
	}
}

// ApplyToQuery applies authentication to the provided query params map
func (a *AuthConfig) ApplyToQuery(queryParams map[string]string) {
	if queryParams == nil {
		return
	}
	if a.Type == AuthTypeAPIKey && a.In == "query" {
		queryParams[a.Name] = a.Key
	}
}

// ApplyToCookies applies authentication to the provided cookies map
func (a *AuthConfig) ApplyToCookies(cookies map[string]string) {
	if cookies == nil {
		return
	}
	if a.Type == AuthTypeAPIKey && a.In == "cookie" {
		cookies[a.Name] = a.Key
	}
}
