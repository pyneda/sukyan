package openapi

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/rs/zerolog/log"
)

var Headers []string

var DefaultCredentials = struct {
	BasicAuthUser string
	BasicAuthPass string
	BearerToken   string
	ApiKey        string
}{
	BasicAuthUser: "admin",
	BasicAuthPass: "admin",
	BearerToken:   "default-bearer-token",
	ApiKey:        "default-api-key",
}

type CheckSecDefsInput struct {
	Doc3          openapi3.T `json:"doc3"`
	BasicAuthUser string     `json:"basic_auth_user"`
	BasicAuthPass string     `json:"basic_auth_pass"`
	BearerToken   string     `json:"bearer_token"`
	ApiKey        string     `json:"api_key"`
}

type SecuritySchemeDetails struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	In          string `json:"in,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Description string `json:"description,omitempty"`
}

type CheckSecDefsOutput struct {
	ApiInQuery           bool                             `json:"api_in_query"`
	ApiKey               string                           `json:"api_key,omitempty"`
	ApiKeyName           string                           `json:"api_key_name,omitempty"`
	Headers              map[string][]string              `json:"headers"`
	SecuritySchemes      map[string]SecuritySchemeDetails `json:"security_schemes"`
	FoundBasicAuth       bool                             `json:"found_basic_auth"`
	FoundBearerToken     bool                             `json:"found_bearer_token"`
	FoundApiKey          bool                             `json:"found_api_key"`
	BasicAuthString      string                           `json:"basic_auth_string,omitempty"`
	HumanReadableSummary string                           `json:"human_readable_summary"`
	Examples             map[string]string                `json:"examples"`
	UsedCredentials      map[string]string                `json:"used_credentials"`
}

func CheckSecDefs(input CheckSecDefsInput) CheckSecDefsOutput {
	output := CheckSecDefsOutput{
		Headers:         make(map[string][]string),
		SecuritySchemes: make(map[string]SecuritySchemeDetails),
		Examples:        make(map[string]string),
		UsedCredentials: make(map[string]string),
	}

	if input.BasicAuthUser == "" {
		input.BasicAuthUser = DefaultCredentials.BasicAuthUser
		output.UsedCredentials["basic_auth_user"] = "default"
	}
	if input.BasicAuthPass == "" {
		input.BasicAuthPass = DefaultCredentials.BasicAuthPass
		output.UsedCredentials["basic_auth_pass"] = "default"
	}
	if input.BearerToken == "" {
		input.BearerToken = DefaultCredentials.BearerToken
		output.UsedCredentials["bearer_token"] = "default"
	}
	if input.ApiKey == "" {
		input.ApiKey = DefaultCredentials.ApiKey
		output.UsedCredentials["api_key"] = "default"
	}

	Headers = []string{}

	if input.Doc3.Components == nil || len(input.Doc3.Components.SecuritySchemes) == 0 {
		log.Warn().Msg("No security schemes detected.")
		output.HumanReadableSummary = "No security mechanisms were found in the API specification."
		return output
	}

	var summaryParts []string

	for mechanism, scheme := range input.Doc3.Components.SecuritySchemes {
		if scheme.Value == nil {
			continue
		}

		details := SecuritySchemeDetails{
			Type:        scheme.Value.Type,
			Name:        scheme.Value.Name,
			In:          scheme.Value.In,
			Scheme:      scheme.Value.Scheme,
			Description: scheme.Value.Description,
		}
		output.SecuritySchemes[mechanism] = details

		switch {
		case scheme.Value.Type == "http" && scheme.Value.Scheme == "basic":
			output.FoundBasicAuth = true
			basicAuth := []byte(fmt.Sprintf("%s:%s", input.BasicAuthUser, input.BasicAuthPass))
			output.BasicAuthString = base64.StdEncoding.EncodeToString(basicAuth)
			authHeader := "Authorization: Basic " + output.BasicAuthString
			output.Headers["Authorization"] = []string{"Basic " + output.BasicAuthString}
			output.Examples["Basic Auth"] = authHeader
			Headers = append(Headers, authHeader)
			summaryParts = append(summaryParts, "Basic Authentication")

		case scheme.Value.Type == "http" && strings.ToLower(scheme.Value.Scheme) == "bearer":
			output.FoundBearerToken = true
			authHeader := "Authorization: Bearer " + input.BearerToken
			output.Headers["Authorization"] = []string{"Bearer " + input.BearerToken}
			output.Examples["Bearer"] = authHeader
			Headers = append(Headers, authHeader)
			summaryParts = append(summaryParts, "Bearer Token Authentication")

		case scheme.Value.Type == "apiKey" && scheme.Value.In == "query":
			output.ApiInQuery = true
			output.ApiKeyName = scheme.Value.Name
			output.ApiKey = input.ApiKey
			output.FoundApiKey = true
			output.Examples["API Key (Query)"] = fmt.Sprintf("?%s=%s", scheme.Value.Name, input.ApiKey)
			summaryParts = append(summaryParts, "API Key in Query Parameter")

		case scheme.Value.Type == "apiKey" && scheme.Value.In == "header":
			output.FoundApiKey = true
			output.ApiKeyName = scheme.Value.Name
			output.ApiKey = input.ApiKey
			if scheme.Value.Name != "" {
				headerValue := scheme.Value.Name + ": " + input.ApiKey
				output.Headers[scheme.Value.Name] = []string{input.ApiKey}
				output.Examples["API Key (Header)"] = headerValue
				Headers = append(Headers, headerValue)
			}
			summaryParts = append(summaryParts, "API Key in Header")
		}
	}

	if len(summaryParts) > 0 {
		output.HumanReadableSummary = fmt.Sprintf("This API supports the following authentication methods: %s", strings.Join(summaryParts, ", "))
	} else {
		output.HumanReadableSummary = "No standard authentication mechanisms were found in the API specification."
	}

	return output
}
