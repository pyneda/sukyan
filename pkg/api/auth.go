package api

import (
	"encoding/base64"
	"net/http"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// ApplyAuthConfig writes a stored auth configuration onto a request.
//
// dynamicToken is a value the caller already resolved from a token manager; it wins
// over the stored token when non-empty. Callers with no token manager pass "" and
// get the stored static credential.
func ApplyAuthConfig(req *http.Request, cfg *db.APIAuthConfig, dynamicToken string) {
	if req == nil || cfg == nil {
		return
	}

	switch cfg.Type {
	case db.APIAuthTypeBasic:
		if cfg.Username != "" || cfg.Password != "" {
			auth := cfg.Username + ":" + cfg.Password
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
		}

	case db.APIAuthTypeBearer:
		token := cfg.Token
		if dynamicToken != "" {
			token = dynamicToken
		}
		if token != "" {
			req.Header.Set("Authorization", bearerPrefix(cfg)+" "+token)
		}

	case db.APIAuthTypeOAuth2:
		token := cfg.Token
		if dynamicToken != "" {
			token = dynamicToken
		}
		if token != "" {
			req.Header.Set("Authorization", bearerPrefix(cfg)+" "+token)
		} else {
			log.Warn().Msg("OAuth2 auth config has no token set, skipping authentication")
		}

	case db.APIAuthTypeAPIKey:
		keyValue := cfg.APIKeyValue
		if dynamicToken != "" {
			keyValue = dynamicToken
		}
		if cfg.APIKeyName == "" || keyValue == "" {
			break
		}
		switch cfg.APIKeyLocation {
		case db.APIKeyLocationHeader:
			req.Header.Set(cfg.APIKeyName, keyValue)
		case db.APIKeyLocationQuery:
			query := req.URL.Query()
			query.Set(cfg.APIKeyName, keyValue)
			req.URL.RawQuery = query.Encode()
		case db.APIKeyLocationCookie:
			req.AddCookie(&http.Cookie{Name: cfg.APIKeyName, Value: keyValue})
		}
	}

	for _, header := range cfg.CustomHeaders {
		req.Header.Set(header.HeaderName, header.HeaderValue)
	}
}

func bearerPrefix(cfg *db.APIAuthConfig) string {
	if cfg.TokenPrefix == "" {
		return "Bearer"
	}
	return cfg.TokenPrefix
}
