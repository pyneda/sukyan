package tokens

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type JWTCrackResult struct {
	Token       string
	TokenModel  *db.JsonWebToken
	Found       bool
	Secret      string
	ElapsedTime float64
	Issue       *db.Issue
	Error       error
}

func CrackJWTAndCreateIssue(history *db.History, token *db.JsonWebToken) (*JWTCrackResult, error) {
	result := &JWTCrackResult{
		Token:      token.Token,
		TokenModel: token,
	}

	// by now using the embedded wordlist is the only option
	crackResult := CrackJWT(token.Token, "", 5, true)
	if crackResult == nil {
		result.Error = fmt.Errorf("failed to crack JWT, received nil response for: %s", token)
		return result, result.Error
	}

	result.Found = crackResult.Found
	result.Secret = crackResult.Secret
	result.ElapsedTime = crackResult.Duration.Seconds()

	if result.Found {
		details := generateIssueDetails(result, history)
		noTaskJob := uint(0)
		issue, err := db.CreateIssueFromHistoryAndTemplate(
			history,
			db.JwtWeakSigningSecretCode,
			details,
			100,
			"",
			history.WorkspaceID,
			history.TaskID,
			&noTaskJob,
			history.ScanID,
			history.ScanJobID,
		)

		if err != nil {
			log.Error().Err(err).
				Str("token", token.Token).
				Str("secret", result.Secret).
				Msg("Failed to create issue for cracked JWT")
			result.Error = fmt.Errorf("failed to create issue: %w", err)
			return result, result.Error
		}

		result.Issue = &issue
		log.Info().
			Str("token", token.Token).
			Str("secret", result.Secret).
			Uint("issue_id", issue.ID).
			Msg("Created issue for cracked JWT")
	}

	result.TokenModel.TestedEmbeddedWordlist = true
	result.TokenModel.Cracked = result.Found
	result.TokenModel.Secret = result.Secret
	if err := db.Connection().UpdateJWT(token.ID, result.TokenModel); err != nil {
		log.Error().Err(err).Uint("token", token.ID).Bool("success", result.Found).Str("secret", result.Secret).Msg("Failed to update JWT with crack result")
		result.Error = fmt.Errorf("failed to update JWT with crack results: %w", err)
		return result, result.Error
	}

	return result, nil
}

// CrackJWTAndCreateIssueFromWebSocket handles JWT cracking specifically for WebSocket context
func CrackJWTAndCreateIssueFromWebSocket(message *db.WebSocketMessage, connection *db.WebSocketConnection, token *db.JsonWebToken) (*JWTCrackResult, error) {
	result := &JWTCrackResult{
		Token:      token.Token,
		TokenModel: token,
	}

	// Use the embedded wordlist for cracking
	crackResult := CrackJWT(token.Token, "", 5, true)
	if crackResult == nil {
		result.Error = fmt.Errorf("failed to crack JWT, received nil response for: %s", token)
		return result, result.Error
	}

	result.Found = crackResult.Found
	result.Secret = crackResult.Secret
	result.ElapsedTime = crackResult.Duration.Seconds()

	if result.Found {
		details := generateWebSocketIssueDetails(result, message, connection)
		noTaskJob := uint(0)

		issue, err := db.CreateIssueFromWebSocketMessage(
			message,
			db.JwtWeakSigningSecretCode,
			details,
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
			log.Error().Err(err).
				Str("token", token.Token).
				Str("secret", result.Secret).
				Msg("Failed to create issue for cracked JWT from WebSocket")
			result.Error = fmt.Errorf("failed to create issue: %w", err)
			return result, result.Error
		}

		result.Issue = &issue
		log.Info().
			Str("token", token.Token).
			Str("secret", result.Secret).
			Uint("issue_id", issue.ID).
			Msg("Created issue for cracked JWT from WebSocket")
	}

	result.TokenModel.TestedEmbeddedWordlist = true
	result.TokenModel.Cracked = result.Found
	result.TokenModel.Secret = result.Secret
	if err := db.Connection().UpdateJWT(token.ID, result.TokenModel); err != nil {
		log.Error().Err(err).Uint("token", token.ID).Bool("success", result.Found).Str("secret", result.Secret).Msg("Failed to update JWT with crack result")
		result.Error = fmt.Errorf("failed to update JWT with crack results: %w", err)
		return result, result.Error
	}

	return result, nil
}

// CrackJWTAndCreateIssueFromWebSocketConnection handles JWT cracking for WebSocket connection headers
func CrackJWTAndCreateIssueFromWebSocketConnection(connection *db.WebSocketConnection, token *db.JsonWebToken) (*JWTCrackResult, error) {
	result := &JWTCrackResult{
		Token:      token.Token,
		TokenModel: token,
	}

	// Use the embedded wordlist for cracking
	crackResult := CrackJWT(token.Token, "", 5, true)
	if crackResult == nil {
		result.Error = fmt.Errorf("failed to crack JWT, received nil response for: %s", token)
		return result, result.Error
	}

	result.Found = crackResult.Found
	result.Secret = crackResult.Secret
	result.ElapsedTime = crackResult.Duration.Seconds()

	if result.Found {
		details := generateWebSocketConnectionIssueDetails(result, connection)
		noTaskJob := uint(0)

		issue, err := db.CreateIssueFromWebSocketConnectionAndTemplate(
			connection,
			db.JwtWeakSigningSecretCode,
			details,
			100,
			"",
			connection.WorkspaceID,
			connection.TaskID,
			&noTaskJob,
			connection.ScanID,
			connection.ScanJobID,
		)

		if err != nil {
			log.Error().Err(err).
				Str("token", token.Token).
				Str("secret", result.Secret).
				Msg("Failed to create issue for cracked JWT from WebSocket connection")
			result.Error = fmt.Errorf("failed to create issue: %w", err)
			return result, result.Error
		}

		result.Issue = &issue
		log.Info().
			Str("token", token.Token).
			Str("secret", result.Secret).
			Uint("issue_id", issue.ID).
			Msg("Created issue for cracked JWT from WebSocket connection")
	}

	result.TokenModel.TestedEmbeddedWordlist = true
	result.TokenModel.Cracked = result.Found
	result.TokenModel.Secret = result.Secret
	if err := db.Connection().UpdateJWT(token.ID, result.TokenModel); err != nil {
		log.Error().Err(err).Uint("token", token.ID).Bool("success", result.Found).Str("secret", result.Secret).Msg("Failed to update JWT with crack result")
		result.Error = fmt.Errorf("failed to update JWT with crack results: %w", err)
		return result, result.Error
	}

	return result, nil
}

func generateIssueDetails(result *JWTCrackResult, history *db.History) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("A JWT token with a weak secret was discovered in a %s request to %s\n\n", history.Method, history.URL))
	sb.WriteString(fmt.Sprintf("The token was successfully cracked in %.2f seconds.\n\n", result.ElapsedTime))
	sb.WriteString("Details:\n")
	if result.TokenModel != nil {
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", result.TokenModel.Algorithm))
	}
	sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n\n", result.Secret))

	return sb.String()
}

func generateWebSocketIssueDetails(result *JWTCrackResult, message *db.WebSocketMessage, connection *db.WebSocketConnection) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("A JWT token with a weak secret was discovered in a WebSocket message to %s\n\n", connection.URL))
	sb.WriteString(fmt.Sprintf("The token was successfully cracked in %.2f seconds.\n\n", result.ElapsedTime))
	sb.WriteString("Details:\n")
	if result.TokenModel != nil {
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", result.TokenModel.Algorithm))
	}
	sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n", result.Secret))
	sb.WriteString(fmt.Sprintf("- Message Direction: %s\n", message.Direction))
	sb.WriteString(fmt.Sprintf("- Message Timestamp: %s\n\n", message.Timestamp.Format("2006-01-02 15:04:05")))

	return sb.String()
}

func generateWebSocketConnectionIssueDetails(result *JWTCrackResult, connection *db.WebSocketConnection) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("A JWT token with a weak secret was discovered in WebSocket connection headers for %s\n\n", connection.URL))
	sb.WriteString(fmt.Sprintf("The token was successfully cracked in %.2f seconds.\n\n", result.ElapsedTime))
	sb.WriteString("Details:\n")
	if result.TokenModel != nil {
		sb.WriteString(fmt.Sprintf("- Algorithm: %s\n", result.TokenModel.Algorithm))
	}
	sb.WriteString(fmt.Sprintf("- Discovered Secret: %s\n\n", result.Secret))

	return sb.String()
}
