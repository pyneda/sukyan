package openapi

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	autoApplyAPIKey    string = "n"
	autoApplyBasicAuth string = "n"
	autoApplyBearer    string = "n"
	basicAuthUser      string = "admin"
	basicAuthPass      string = "admin"
	basicAuth          []byte
	basicAuthString    string
	bearerToken        string
	Headers            []string
)

func CheckSecDefs(doc3 openapi3.T) (apiInQuery bool, apiKey string, apiKeyName string) {

	if doc3.Components != nil && len(doc3.Components.SecuritySchemes) > 0 {
		log.Info().Int("security schemes", len(doc3.Components.SecuritySchemes)).Msg("Security schemes detected.")
	} else {
		log.Warn().Msg("No security schemes detected.")
		return false, "", ""
	}

	for mechanism := range doc3.Components.SecuritySchemes {
		if doc3.Components.SecuritySchemes[mechanism].Value == nil {
			log.Warn().Str("mechanism", mechanism).Msg("Unsupported security scheme.")
			return false, "", ""
		}
		if doc3.Components.SecuritySchemes[mechanism].Value.Scheme != "" {
			fmt.Printf("    - %s (%s)\n", mechanism, doc3.Components.SecuritySchemes[mechanism].Value.Scheme)
		} else {
			fmt.Printf("    - %s\n", mechanism)
		}

		if doc3.Components.SecuritySchemes[mechanism].Value.Type == "http" {
			if doc3.Components.SecuritySchemes[mechanism].Value.Scheme == "basic" {
				log.Info().Str("mechanism", mechanism).Msg("Basic Auth is accepted.")
				basicAuth = []byte(basicAuthUser + ":" + basicAuthPass)
				basicAuthString = base64.StdEncoding.EncodeToString(basicAuth)
				log.Info().Str("credentials", basicAuthString).Msg("Using Basic Auth credentials.")
				Headers = append(Headers, "Authorization: Basic "+basicAuthString)

			} else if strings.ToLower(doc3.Components.SecuritySchemes[mechanism].Value.Scheme) == "bearer" {
				log.Warn().Str("mechanism", mechanism).Msg("Bearer token is accepted.")
			}
		} else if doc3.Components.SecuritySchemes[mechanism].Value.Type == "apiKey" && doc3.Components.SecuritySchemes[mechanism].Value.In == "query" {
			apiInQuery = true
			log.Info().Msg("An API key can be provided via the query string.")
			apiKeyName = doc3.Components.SecuritySchemes[mechanism].Value.Name
			log.Info().Str("name", apiKeyName).Str("value", apiKey).Msg("Using the API key in the query string.")
		} else if doc3.Components.SecuritySchemes[mechanism].Value.Type == "apiKey" && doc3.Components.SecuritySchemes[mechanism].Value.In == "header" {
			if mechanism == "bearer" {
				log.Info().Msg("Bearer token is accepted.")
				Headers = append(Headers, "Authorization: Bearer "+bearerToken)
				log.Info().Str("token", bearerToken).Msg("Using Bearer token.")

			} else {
				log.Info().Str("mechanism", mechanism).Str("header", doc3.Components.SecuritySchemes[mechanism].Value.Name).Msg("API key can be provided via the header.")
				apiKeyName = doc3.Components.SecuritySchemes[mechanism].Value.Name
				log.Info().Str("name", apiKeyName).Str("value", apiKey).Msg("Using the API key in the header.")
				Headers = append(Headers, doc3.Components.SecuritySchemes[mechanism].Value.Name+": "+apiKey)
			}
		}
	}
	return apiInQuery, apiKey, apiKeyName
}
