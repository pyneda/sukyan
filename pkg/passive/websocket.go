package passive

import (
	"context"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/tokens"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc"
)

// WebSocketPassiveScanResult contains the results of passive WebSocket scanning
type WebSocketPassiveScanResult struct {
	ConnectionID uint       `json:"connection_id"`
	MessageCount int        `json:"message_count"`
	Issues       []db.Issue `json:"issues"`
}

// ScanWebSocketConnection performs passive scans on a WebSocket connection and returns found issues
func ScanWebSocketConnection(connection *db.WebSocketConnection) *WebSocketPassiveScanResult {
	log.Info().Uint("connection_id", connection.ID).Str("url", connection.URL).Msg("Starting passive scan on WebSocket connection")

	result := &WebSocketPassiveScanResult{
		ConnectionID: connection.ID,
		Issues:       []db.Issue{},
	}

	headerIssues := ScanWebSocketConnectionHeaders(connection)
	result.Issues = append(result.Issues, headerIssues...)

	messages, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		ConnectionID: connection.ID,
	})
	if err != nil {
		log.Error().Err(err).Uint("connection_id", connection.ID).Msg("Failed to load WebSocket messages for passive scanning")
		return result
	}

	result.MessageCount = len(messages)

	for _, message := range messages {
		messageIssues := ScanWebSocketMessage(&message, connection)
		result.Issues = append(result.Issues, messageIssues...)
	}

	log.Info().Uint("connection_id", connection.ID).Str("url", connection.URL).Int("messages_scanned", len(messages)).Int("issues_found", len(result.Issues)).Msg("Completed passive scan on WebSocket connection")
	return result
}

// ScanWebSocketMessage performs passive scans on a single WebSocket message and returns found issues
func ScanWebSocketMessage(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	if message.PayloadData == "" {
		return []db.Issue{}
	}

	log.Info().Uint("message_id", message.ID).Uint("connection_id", message.ConnectionID).Msg("Starting passive scan on WebSocket message")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var allIssues []db.Issue

	scanFunctions := []struct {
		name    string
		timeout time.Duration
		fn      func() []db.Issue
	}{
		{"DatabaseErrorScan", 5 * time.Second, func() []db.Issue { return DatabaseErrorScanWS(message, connection) }},
		{"LeakedApiKeysScan", 5 * time.Second, func() []db.Issue { return LeakedApiKeysScanWS(message, connection) }},
		{"PrivateIPScan", 5 * time.Second, func() []db.Issue { return PrivateIPScanWS(message, connection) }},
		{"JwtDetectionScan", 5 * time.Second, func() []db.Issue { return JwtDetectionScanWS(message, connection) }},
		{"PrivateKeyScan", 5 * time.Second, func() []db.Issue { return PrivateKeyScanWS(message, connection) }},
		{"DBConnectionStringScan", 5 * time.Second, func() []db.Issue { return DBConnectionStringScanWS(message, connection) }},
		{"EmailAddressScan", 5 * time.Second, func() []db.Issue { return EmailAddressScanWS(message, connection) }},
		{"StorageBucketDetectionScan", 5 * time.Second, func() []db.Issue { return StorageBucketDetectionScanWS(message, connection) }},
		{"SessionTokenScan", 5 * time.Second, func() []db.Issue { return SessionTokenScanWS(message, connection) }},
	}

	var wg conc.WaitGroup

	for _, scanFunc := range scanFunctions {
		scanFunc := scanFunc // capture loop variable
		wg.Go(func() {
			scanCtx, scanCancel := context.WithTimeout(ctx, scanFunc.timeout)
			defer scanCancel()

			resultChan := make(chan []db.Issue, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						log.Error().
							Interface("panic", r).
							Str("scan_function", scanFunc.name).
							Uint("message_id", message.ID).
							Str("stack_trace", string(stack)).
							Msg("Panic recovered in passive WebSocket scan function")
					}
				}()
				issues := scanFunc.fn()
				resultChan <- issues
			}()

			select {
			case issues := <-resultChan:
				if len(issues) > 0 {
					mu.Lock()
					allIssues = append(allIssues, issues...)
					mu.Unlock()
				}
			case <-scanCtx.Done():
				log.Warn().Str("scan_function", scanFunc.name).Dur("timeout", scanFunc.timeout).Uint("message_id", message.ID).Msg("WebSocket passive scan function timed out")
			}
		})
	}
	log.Info().Uint("message_id", message.ID).Msg("Waiting for all passive WebSocket scan functions to complete")
	wg.Wait()

	log.Info().Uint("message_id", message.ID).Uint("connection_id", connection.ID).Int("issues_found", len(allIssues)).Msg("Completed passive scan on WebSocket message")

	return allIssues
}

// ScanWebSocketConnectionHeaders scans the WebSocket connection headers for security issues and returns found issues
func ScanWebSocketConnectionHeaders(connection *db.WebSocketConnection) []db.Issue {
	var issues []db.Issue

	requestHeaders, err := connection.GetRequestHeadersAsMap()
	if err == nil {
		requestIssues := checkWebSocketHeadersForJwt(connection, requestHeaders, "request")
		issues = append(issues, requestIssues...)
	}

	responseHeaders, err := connection.GetResponseHeadersAsMap()
	if err == nil {
		responseIssues := checkWebSocketHeadersForJwt(connection, responseHeaders, "response")
		issues = append(issues, responseIssues...)
	}

	return issues
}

// DatabaseErrorScanWSAndReturnIssues scans WebSocket message payload for database errors and returns found issues
func DatabaseErrorScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	if message == nil || connection == nil {
		log.Warn().Msg("Nil message or connection in passive.DatabaseErrorScanWS")
		return []db.Issue{}
	}

	match := SearchDatabaseErrors(message.PayloadData)
	if match != nil {
		errorDescription := fmt.Sprintf("Discovered database error in WebSocket message: \n - Database type: %s\n - Error: %s", match.DatabaseName, match.MatchStr)

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.DatabaseErrorsCode,
			errorDescription,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create database error issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// LeakedApiKeysScanWS scans WebSocket message payload for leaked API keys and returns found issues
func LeakedApiKeysScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	if message == nil || connection == nil {
		log.Warn().Msg("Nil message or connection in passive.LeakedApiKeysScanWS")
		return []db.Issue{}
	}

	var sb strings.Builder
	matched := false

	for patternName, pattern := range apiKeysPatternsMap {
		if pattern == nil {
			continue
		}
		matches := pattern.FindAllString(message.PayloadData, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("\nDiscovered %s in WebSocket message", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
		}
	}

	if matched {
		details := sb.String()

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.ExposedApiCredentialsCode,
			details,
			80,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create API key issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// PrivateIPScanWS scans WebSocket message payload for private IP addresses and returns found issues
func PrivateIPScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	// Add nil checks to prevent panics
	if message == nil || connection == nil {
		log.Warn().Msg("Nil message or connection in passive.PrivateIPScanWS")
		return []db.Issue{}
	}

	if privateIPRegex == nil {
		log.Error().Msg("privateIPRegex is nil in passive.PrivateIPScanWS")
		return []db.Issue{}
	}

	matches := privateIPRegex.FindAllString(message.PayloadData, -1)
	// Filter out the connection host IP if it's in the matches
	host, _ := lib.GetHostFromURL(connection.URL)
	matches = lib.FilterOutString(matches, host)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered Internal IP addresses in WebSocket message:")
		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredIPs := sb.String()

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.PrivateIpsCode,
			discoveredIPs,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create private IP issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// JwtDetectionScanWS scans WebSocket message payload for JWT tokens and returns found issues
func JwtDetectionScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	// Add nil checks to prevent panics
	if message == nil || connection == nil {
		log.Warn().Msg("Nil message or connection in passive.JwtDetectionScanWS")
		return []db.Issue{}
	}

	if jwtRegex == nil {
		log.Error().Msg("jwtRegex is nil in passive.JwtDetectionScanWS")
		return []db.Issue{}
	}

	matches := jwtRegex.FindAllString(message.PayloadData, -1)
	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Detected potential JWTs in WebSocket message:")
		log.Info().Strs("matches", matches).Uint("message_id", message.ID).Msg("Found JWTs in WebSocket message")

		var issues []db.Issue

		for _, match := range matches {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
			// Create or get existing JWT record and attempt to crack it
			jwt, err := db.Connection().GetOrCreateJWTFromTokenAndWebSocketMessage(match, message.ID)
			if err != nil {
				log.Err(err).Msg("Failed to get or create JWT from WebSocket message")
				continue
			}

			// Handle JWT cracking and create additional issues if needed
			crackIssues := attemptToCrackJwtFromWebSocketIfRequired(message, connection, jwt)
			issues = append(issues, crackIssues...)
		}

		details := sb.String()

		// Create the main JWT detection issue
		jwtIssue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.JwtDetectedCode,
			details,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create JWT detection issue from WebSocket message")
		} else {
			issues = append(issues, jwtIssue)
		}

		return issues
	}
	return []db.Issue{}
}

// attemptToCrackJwtFromWebSocketIfRequired attempts to crack JWT found in WebSocket messages and returns found issues
func attemptToCrackJwtFromWebSocketIfRequired(message *db.WebSocketMessage, connection *db.WebSocketConnection, jwt *db.JsonWebToken) []db.Issue {
	if jwt != nil && !jwt.TestedEmbeddedWordlist && !jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Uint("message_id", message.ID).Msg("Token seen for the first time in WebSocket, attempting to crack it with embedded wordlist")
		crackResult, err := tokens.CrackJWTAndCreateIssueFromWebSocket(message, connection, jwt)
		if err != nil {
			log.Err(err).Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Failed to crack JWT from WebSocket")
		} else if crackResult.Found {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Str("secret", crackResult.Secret).Msg("JWT cracked from WebSocket")
			if crackResult.Issue != nil {
				return []db.Issue{*crackResult.Issue}
			}
		} else {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("JWT secret could not be found in WebSocket message")
		}
	} else if jwt != nil && jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Token already cracked, creating issue for WebSocket discovery")

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("A JWT token that had been cracked previously has been discovered in a WebSocket message to %s\n\n", connection.URL))
		sb.WriteString("Details:\n")
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", jwt.Algorithm))
		sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n", jwt.Secret))
		sb.WriteString(fmt.Sprintf("- Message Direction: %s\n", message.Direction))
		sb.WriteString(fmt.Sprintf("- Message Timestamp: %s\n\n", message.Timestamp.Format("2006-01-02 15:04:05")))

		noTaskJob := uint(0)
		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.JwtWeakSigningSecretCode,
			sb.String(),
			100,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&noTaskJob,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Str("token", jwt.Token).Msg("Failed to create issue for already cracked JWT from WebSocket")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// attemptToCrackJwtFromWebSocketConnectionIfRequired attempts to crack JWT found in WebSocket connection headers and returns found issues
func attemptToCrackJwtFromWebSocketConnectionIfRequired(connection *db.WebSocketConnection, jwt *db.JsonWebToken) []db.Issue {
	if jwt != nil && !jwt.TestedEmbeddedWordlist && !jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Uint("connection_id", connection.ID).Msg("Token seen for the first time in WebSocket connection, attempting to crack it with embedded wordlist")
		crackResult, err := tokens.CrackJWTAndCreateIssueFromWebSocketConnection(connection, jwt)
		if err != nil {
			log.Err(err).Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Failed to crack JWT from WebSocket connection")
		} else if crackResult.Found {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Str("secret", crackResult.Secret).Msg("JWT cracked from WebSocket connection")
		} else {
			log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("JWT secret could not be found in WebSocket connection")
		}
	} else if jwt != nil && jwt.Cracked {
		log.Info().Uint("token_id", jwt.ID).Str("token", jwt.Token).Msg("Token already cracked, creating issue for WebSocket connection discovery")

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("A JWT token that had been cracked previously has been discovered in WebSocket connection headers for %s\n\n", connection.URL))
		sb.WriteString("Details:\n")
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", jwt.Algorithm))
		sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n\n", jwt.Secret))

		issue, err := db.CreateWebSocketIssue(db.WebSocketIssueOptions{
			Connection:  connection,
			Code:        db.JwtWeakSigningSecretCode,
			Details:     sb.String(),
			Confidence:  100,
			WorkspaceID: connection.WorkspaceID,
			TaskID:      connection.TaskID,
			ScanID:      connection.ScanID,
			ScanJobID:   connection.ScanJobID,
		})
		if err != nil {
			log.Error().Err(err).Str("token", jwt.Token).Msg("Failed to create issue for already cracked JWT from WebSocket connection")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// checkWebSocketHeadersForJwtAndReturnIssues checks WebSocket connection headers for JWT tokens and returns found issues
func checkWebSocketHeadersForJwt(connection *db.WebSocketConnection, headers map[string][]string, headerType string) []db.Issue {
	var sb strings.Builder
	var issues []db.Issue

	for key, values := range headers {
		for _, value := range values {
			matches := jwtRegex.FindAllString(value, -1)
			if len(matches) > 0 {
				log.Info().Strs("matches", matches).Str("header_type", headerType).Uint("connection_id", connection.ID).Msg("Found JWTs in WebSocket headers")

				sb.WriteString(fmt.Sprintf("Detected potential JWTs in %s %s header:", headerType, key))
				for _, match := range matches {
					sb.WriteString(fmt.Sprintf("\n - %s", match))
					// Create or get existing JWT record and attempt to crack it
					jwt, err := db.Connection().GetOrCreateJWTFromTokenAndWebSocketConnection(match, connection.ID)
					if err != nil {
						log.Err(err).Msg("Failed to get or create JWT from WebSocket connection headers")
						continue
					}

					// Handle JWT cracking and collect additional issues
					crackIssues := attemptToCrackJwtFromWebSocketConnectionIfRequired(connection, jwt)
					issues = append(issues, crackIssues...)
				}
			}
		}
	}

	details := sb.String()
	if details != "" {
		jwtIssue, err := db.CreateWebSocketIssue(db.WebSocketIssueOptions{
			Connection:  connection,
			Code:        db.JwtDetectedCode,
			Details:     details,
			Confidence:  90,
			WorkspaceID: connection.WorkspaceID,
			TaskID:      connection.TaskID,
			ScanID:      connection.ScanID,
			ScanJobID:   connection.ScanJobID,
		})
		if err != nil {
			log.Error().Err(err).Uint("connection_id", connection.ID).Msg("Failed to create JWT detection issue from WebSocket headers")
		} else {
			issues = append(issues, jwtIssue)
		}
	}

	return issues
}

// PrivateKeyScanWS scans WebSocket message payload for private keys and returns found issues
func PrivateKeyScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	if message == nil || connection == nil {
		log.Warn().Msg("Nil message or connection in passive.PrivateKeyScanWS")
		return []db.Issue{}
	}

	type KeyMatch struct {
		Type  string
		Regex *regexp.Regexp
	}
	keyMatches := []KeyMatch{
		{"RSA", rsaPrivateKeyRegex},
		{"DSA", dsaPrivateKeyRegex},
		{"EC", ecPrivateKeyRegex},
		{"OpenSSH", opensshPrivateKeyRegex},
		{"PEM", pemPrivateKeyRegex},
	}

	var issues []db.Issue

	for _, keyMatch := range keyMatches {
		matches := keyMatch.Regex.FindAllString(message.PayloadData, -1)
		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Discovered %s Private Key(s) in WebSocket message:", keyMatch.Type))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n\n%s", match))
			}
			discoveredKeys := sb.String()

			issue, err := db.CreateIssueFromWebSocketMessage(
				message,
				db.PrivateKeysCode,
				discoveredKeys,
				90,
				"",
				connection.WorkspaceID,
				connection.TaskID,
				&defaultTaskJobID,
				connection.ScanID,
				connection.ScanJobID,
				&connection.ID,
				connection.UpgradeRequestID,
			)
			if err != nil {
				log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create private key issue from WebSocket message")
			} else {
				issues = append(issues, issue)
			}
		}
	}

	return issues
}

// DBConnectionStringScanWS scans WebSocket message payload for database connection strings and returns found issues
func DBConnectionStringScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	connectionStringRegexes := []*regexp.Regexp{
		mongoDBConnectionStringRegex,
		postgreSQLConnectionStringRegex,
		postGISConnectionStringRegex,
		mySQLConnectionStringRegex,
		msSQLConnectionStringRegex,
		oracleConnectionStringRegex,
		sqliteConnectionStringRegex,
		redisConnectionStringRegex,
		rabbitMQConnectionStringRegex,
		cassandraConnectionStringRegex,
		neo4jConnectionStringRegex,
		couchDBConnectionStringRegex,
		influxDBConnectionStringRegex,
		memcachedConnectionStringRegex,
	}

	for _, regex := range connectionStringRegexes {
		matches := regex.FindAllString(message.PayloadData, -1)

		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString("Discovered database connection strings in WebSocket message:")
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
			discoveredStrings := sb.String()

			issue, err := db.CreateIssueFromWebSocketMessage(
				message,
				db.DbConnectionStringsCode,
				discoveredStrings,
				90,
				"",
				connection.WorkspaceID,
				connection.TaskID,
				&defaultTaskJobID,
				connection.ScanID,
				connection.ScanJobID,
				&connection.ID,
				connection.UpgradeRequestID,
			)
			if err != nil {
				log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create DB connection string issue from WebSocket message")
				return []db.Issue{}
			}

			return []db.Issue{issue}
		}
	}
	return []db.Issue{}
}

// EmailAddressScanWS scans WebSocket message payload for email addresses and returns found issues
func EmailAddressScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	matches := emailRegex.FindAllString(message.PayloadData, -1)

	if len(matches) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered email addresses in WebSocket message:")
		for _, match := range lib.GetUniqueItems(matches) {
			sb.WriteString(fmt.Sprintf("\n - %s", match))
		}
		discoveredEmails := sb.String()

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.EmailAddressesCode,
			discoveredEmails,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create email address issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// StorageBucketDetectionScanWS scans WebSocket message payload for storage bucket references and returns found issues
func StorageBucketDetectionScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	var sb strings.Builder
	matched := false

	// Detect buckets in URLs within the message payload
	for patternName, pattern := range bucketsURlsPatternsMap {
		matches := pattern.FindAllString(message.PayloadData, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("Discovered %s bucket URLs in WebSocket message:", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
		}
	}

	// Detect bucket errors in message payload
	for patternName, pattern := range bucketBodyPatternsMap {
		matches := pattern.FindAllString(message.PayloadData, -1)

		if len(matches) > 0 {
			matched = true
			sb.WriteString(fmt.Sprintf("\nDiscovered %s bucket errors in WebSocket message:", patternName))
			for _, match := range matches {
				sb.WriteString(fmt.Sprintf("\n - %s", match))
			}
		}
	}

	if matched {
		details := sb.String()

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.StorageBucketDetectedCode,
			details,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create storage bucket issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// SessionTokenScanWS scans WebSocket message payload for session tokens or auth data and returns found issues
func SessionTokenScanWS(message *db.WebSocketMessage, connection *db.WebSocketConnection) []db.Issue {
	// look for token-like patterns in the message payload (JSON keys, etc.)

	tokenPatterns := []string{
		`"(?:session_?token|auth_?token|access_?token|api_?key|bearer_?token|jwt_?token|id_?token)"\s*:\s*"([^"]+)"`,
		`"(?:session|token|auth|access|api_key|bearer|jwt|authorization)"\s*:\s*"([^"]+)"`,
		`(?:token|auth|session|key)\s*[=:]\s*([a-zA-Z0-9_\-\.]{20,})`,
	}

	var foundTokens []string
	for _, pattern := range tokenPatterns {
		regex := regexp.MustCompile(`(?i)` + pattern)
		matches := regex.FindAllStringSubmatch(message.PayloadData, -1)
		for _, match := range matches {
			if len(match) > 1 {
				foundTokens = append(foundTokens, match[1])
			}
		}
	}

	if len(foundTokens) > 0 {
		var sb strings.Builder
		sb.WriteString("Discovered potential session tokens/credentials in WebSocket message:")
		for _, token := range foundTokens {
			sb.WriteString(fmt.Sprintf("\n - %s", token))
		}
		details := sb.String()

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.SessionTokenInWebsocketCode,
			details,
			90,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&defaultTaskJobID,
			connection.ScanID,
			connection.ScanJobID,
			&connection.ID,
			connection.UpgradeRequestID,
		)
		if err != nil {
			log.Error().Err(err).Uint("message_id", message.ID).Msg("Failed to create session token issue from WebSocket message")
			return []db.Issue{}
		}

		return []db.Issue{issue}
	}
	return []db.Issue{}
}

// ScanWebSocketConnectionWithDeduplication performs passive scans on a WebSocket connection with deduplication
func ScanWebSocketConnectionWithDeduplication(connection *db.WebSocketConnection, deduplicationManager *http_utils.WebSocketDeduplicationManager) *WebSocketPassiveScanResult {
	log.Info().Uint("connection_id", connection.ID).Str("url", connection.URL).Msg("Starting passive scan on WebSocket connection with deduplication")

	result := &WebSocketPassiveScanResult{
		ConnectionID: connection.ID,
		Issues:       []db.Issue{},
	}

	headerIssues := ScanWebSocketConnectionHeaders(connection)
	result.Issues = append(result.Issues, headerIssues...)

	messages, _, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		ConnectionID: connection.ID,
	})
	if err != nil {
		log.Error().Err(err).Uint("connection_id", connection.ID).Msg("Failed to load WebSocket messages for passive scanning")
		return result
	}

	result.MessageCount = len(messages)
	scannedMessages := 0
	skippedMessages := 0

	for _, message := range messages {
		if deduplicationManager != nil && !deduplicationManager.ShouldScanMessage(connection.ID, &message) {
			skippedMessages++
			log.Debug().
				Uint("connection_id", connection.ID).
				Uint("message_id", message.ID).
				Msg("Skipping WebSocket message due to passive deduplication rules")
			continue
		}

		messageIssues := ScanWebSocketMessage(&message, connection)
		result.Issues = append(result.Issues, messageIssues...)

		if deduplicationManager != nil {
			deduplicationManager.MarkMessageAsScanned(connection.ID, &message)
		}
		scannedMessages++
	}

	log.Info().
		Uint("connection_id", connection.ID).
		Str("url", connection.URL).
		Int("messages_scanned", scannedMessages).
		Int("messages_skipped", skippedMessages).
		Int("issues_found", len(result.Issues)).
		Msg("Completed passive scan on WebSocket connection with deduplication")

	return result
}
