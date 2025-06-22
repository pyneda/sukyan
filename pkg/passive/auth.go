package passive

import (
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/tokens"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func AuthenticationScan(item *db.History) {
	log.Debug().Str("url", item.URL).Msg("Passive scanning  for Authentication mechanisms")
	headers, err := item.ResponseHeaders()
	if err != nil {
		log.Warn().Err(err).Uint("history_id", item.ID).Msg("Failed to get response headers")
		return
	}

	authHeaders, exists := getHeaders("WWW-Authenticate", headers)
	if !exists {
		log.Debug().Uint("history_id", item.ID).Msg("No WWW-Authenticate header found")
		return
	}

	for _, authHeader := range authHeaders {
		authType := extractAuthType(authHeader)
		switch strings.ToLower(authType) {
		case "basic":
			handleBasicAuth(item, authHeader)
		case "digest":
			handleDigestAuth(item, authHeader)
		case "bearer":
			handleBearerAuth(item, authHeader)
		case "ntlm":
			handleNTLMAuth(item, authHeader)
		case "negotiate":
			handleNegotiateAuth(item, authHeader)
		case "mutual":
			handleMutualAuth(item, authHeader)
		default:
			handleUnknownAuth(item, authType, authHeader)
		}
	}
}

// handleBasicAuth processes Basic Authentication headers
func handleBasicAuth(item *db.History, authHeader string) {
	realm := extractRealm(authHeader)

	var details string
	var severity string
	confidence := 90

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.High.String()
		details = "HTTP Basic Authentication detected over an unencrypted connection.\n"
		if realm != "" {
			details += "- Realm: " + realm + "\n\n"
		}
	} else {
		severity = db.Info.String()
		if realm != "" {
			details += "- Realm: " + realm + "\n\n"
		}
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.BasicAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)

	if viper.GetBool("auth.bruteforce.enabled") {
		go func() {
			_, err := tokens.BruteforceAuthAndCreateIssue(item, authHeader, tokens.AuthTypeBasic)
			if err != nil {
				log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to bruteforce basic auth")
			}
		}()
	}
}

// handleDigestAuth processes Digest Authentication headers
func handleDigestAuth(item *db.History, authHeader string) {
	realm := tokens.ExtractDigestParam(authHeader, "realm")
	nonce := tokens.ExtractDigestParam(authHeader, "nonce")
	qop := tokens.ExtractDigestParam(authHeader, "qop")
	algorithm := tokens.ExtractDigestParam(authHeader, "algorithm")

	var details string
	var severity string
	confidence := 90

	details = "Parameters detected:\n"
	if realm != "" {
		details += "- Realm: " + realm + "\n"
	}
	if algorithm != "" {
		details += "- Algorithm: " + algorithm + "\n"
	}
	if qop != "" {
		details += "- Quality of Protection (QOP): " + qop + "\n"
	}
	if nonce != "" {
		details += "- Nonce: " + nonce + "\n"
	}

	usesWeakAlgorithm := false
	// Check for weak algorithms
	if algorithm != "" && (strings.Contains(strings.ToLower(algorithm), "md5")) {
		details += "\nWARNING: Using MD5 algorithm which is considered cryptographically weak. "
		details += "MD5 has known vulnerabilities and collision attacks. Consider using stronger algorithms if available.\n"
		usesWeakAlgorithm = true
	}

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		if usesWeakAlgorithm {
			severity = db.High.String()
		} else {
			severity = db.Medium.String()
		}

		details += "\nDigest authentication is being used over HTTP. While more secure than Basic auth, "
		details += "it can still be vulnerable to man-in-the-middle attacks when used over unencrypted connections. "
	} else {
		if usesWeakAlgorithm {
			severity = db.Low.String()
		} else {
			severity = db.Info.String()
		}
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.DigestAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)

	if viper.GetBool("auth.bruteforce.enabled") {
		go func() {
			_, err := tokens.BruteforceAuthAndCreateIssue(item, authHeader, tokens.AuthTypeDigest)
			if err != nil {
				log.Debug().Err(err).Uint("history_id", item.ID).Msg("Failed to bruteforce digest auth")
			}
		}()
	}
}

// handleBearerAuth processes Bearer (OAuth2) Authentication headers
func handleBearerAuth(item *db.History, authHeader string) {
	// Extract any parameters if present
	realm := extractOAuthParam(authHeader, "realm")
	error := extractOAuthParam(authHeader, "error")
	errorDesc := extractOAuthParam(authHeader, "error_description")

	var details string
	var severity string
	confidence := 90

	details = "Bearer Authentication (OAuth 2.0) detected. "
	details += "This is a token-based authentication scheme commonly used with OAuth 2.0 and JWT. "

	if realm != "" || error != "" || errorDesc != "" {
		details += "\n\nParameters detected:\n"
		if realm != "" {
			details += "- Realm: " + realm + "\n"
		}
		if error != "" {
			details += "- Error: " + error + "\n"
		}
		if errorDesc != "" {
			details += "- Error Description: " + errorDesc + "\n"
		}
	}

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.High.String()
		details += "\nBearer tokens are being transmitted over an unencrypted HTTP connection. "
		details += "This exposes the tokens to interception, potentially allowing attackers to impersonate users. "
	} else {
		severity = db.Info.String()
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.BearerAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)
}

// handleNTLMAuth processes NTLM Authentication headers
func handleNTLMAuth(item *db.History, authHeader string) {
	var details string
	var severity string
	confidence := 90

	// Check if NTLM has additional data (indicating which version might be in use)
	hasNTLMData := len(strings.TrimSpace(strings.Replace(authHeader, "NTLM", "", 1))) > 0
	if hasNTLMData {
		details += "The response contains NTLM authentication data, which may indicate an ongoing NTLM authentication exchange."
	}

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.High.String()
		details += "\n\nWARNING: NTLM authentication is being used over an unencrypted HTTP connection. "
		details += "This exposes the authentication exchange to potential interception and increases the risk of attacks like NTLM relay. "
	} else {
		severity = db.Medium.String()
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.NtlmAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)
}

// handleNegotiateAuth processes Negotiate/Kerberos Authentication headers
func handleNegotiateAuth(item *db.History, authHeader string) {
	var details string
	var severity string
	confidence := 90

	// Check if Negotiate has additional data
	hasData := len(strings.TrimSpace(strings.Replace(authHeader, "Negotiate", "", 1))) > 0
	if hasData {
		details += "The response contains authentication data, which may indicate an ongoing authentication exchange."
	}

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.Medium.String()
		details += "\n\nWARNING: Negotiate authentication is being used over an unencrypted HTTP connection. "
		details += "This exposes the authentication exchange to potential interception. "
	} else {
		severity = db.Info.String()
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.NegotiateAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)
}

// handleMutualAuth processes Mutual Authentication headers
func handleMutualAuth(item *db.History, authHeader string) {
	algorithm := extractMutualParam(authHeader, "algorithm")
	realm := extractMutualParam(authHeader, "realm")

	var details string
	var severity string
	confidence := 90

	if realm != "" || algorithm != "" {
		details += "\n\nParameters detected:\n"
		if realm != "" {
			details += "- Realm: " + realm + "\n"
		}
		if algorithm != "" {
			details += "- Algorithm: " + algorithm + "\n"
		}
	}

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.Medium.String()
		details += "\n\nWARNING: Mutual authentication is being used over an unencrypted HTTP connection. "
		details += "While mutual authentication provides strong entity authentication, the lack of transport encryption "
		details += "could still expose sensitive data. Consider using HTTPS to protect data in transit."
	} else {
		severity = db.Info.String()
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.MutualAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)
}

// handleUnknownAuth processes unknown Authentication headers
func handleUnknownAuth(item *db.History, authType string, authHeader string) {
	var details string
	var severity string
	confidence := 80

	details = "Unknown, unhandled or custom authentication method detected: " + authType + ".\n\n"
	details += "Full authentication header: " + authHeader

	if strings.HasPrefix(strings.ToLower(item.URL), "http://") {
		severity = db.Medium.String()
		details += "\n\nWARNING: This authentication method is being used over an unencrypted HTTP connection. "
		details += "Consider using HTTPS to protect authentication credentials and tokens in transit."
	} else {
		severity = db.Info.String()
	}

	db.CreateIssueFromHistoryAndTemplate(
		item,
		db.UnknownAuthDetectedCode,
		details,
		confidence,
		severity,
		item.WorkspaceID,
		item.TaskID,
		&defaultTaskJobID,
	)
}

// Common helper function to extract specific headers with case-insensitive matching
func getHeaders(headerName string, headers map[string][]string) ([]string, bool) {
	for key, values := range headers {
		if strings.EqualFold(key, headerName) {
			return values, true
		}
	}
	return nil, false
}

// Extract the authentication type from the header
func extractAuthType(authHeader string) string {
	// Get the first part before any space or parameters
	parts := strings.SplitN(authHeader, " ", 2)
	return strings.TrimSpace(parts[0])
}

func extractRealm(authHeader string) string {
	// Extract realm value from header like: Basic realm="Login Required"
	realmPrefix := "realm="
	realmPrefixIndex := strings.Index(strings.ToLower(authHeader), strings.ToLower(realmPrefix))
	if realmPrefixIndex == -1 {
		// Try with spaces around equals
		realmPrefix = "realm ="
		realmPrefixIndex = strings.Index(strings.ToLower(authHeader), strings.ToLower(realmPrefix))
		if realmPrefixIndex == -1 {
			// Try with space after equals
			realmPrefix = "realm= "
			realmPrefixIndex = strings.Index(strings.ToLower(authHeader), strings.ToLower(realmPrefix))
			if realmPrefixIndex == -1 {
				// Try with spaces on both sides
				realmPrefix = "realm = "
				realmPrefixIndex = strings.Index(strings.ToLower(authHeader), strings.ToLower(realmPrefix))
				if realmPrefixIndex == -1 {
					return ""
				}
			}
		}
	}

	valueStart := realmPrefixIndex + len(realmPrefix)
	value := authHeader[valueStart:]
	value = strings.TrimSpace(value)

	// Handle quoted values
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		quote := value[0:1]
		endQuoteIndex := strings.Index(value[1:], quote)
		if endQuoteIndex != -1 {
			return value[1 : endQuoteIndex+1]
		}
	}

	// Handle unquoted values (find next comma or end)
	endIndex := strings.Index(value, ",")
	if endIndex == -1 {
		return value
	}

	return value[:endIndex]
}

// Extract parameter from OAuth auth header
func extractOAuthParam(authHeader string, param string) string {
	return tokens.ExtractDigestParam(authHeader, param)
}

// Extract parameter from Mutual auth header
func extractMutualParam(authHeader string, param string) string {
	return tokens.ExtractDigestParam(authHeader, param)
}
