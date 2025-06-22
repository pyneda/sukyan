package tokens

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type AuthBruteforceDBResult struct {
	BruteforceResult *AuthBruteforceResult
	Issue            *db.Issue
	Error            error
}

func BruteforceAuthAndCreateIssue(historyItem *db.History, authHeader string, authType AuthType) (*AuthBruteforceDBResult, error) {
	result := &AuthBruteforceDBResult{
		BruteforceResult: &AuthBruteforceResult{},
	}

	config := CreateDefaultAuthBruteforceConfig(authType)

	bruteforceResult := BruteforceAuth(historyItem, authHeader, config)
	result.BruteforceResult = bruteforceResult

	if bruteforceResult.Error != nil {
		result.Error = fmt.Errorf("brute force failed: %w", bruteforceResult.Error)
		return result, result.Error
	}

	if bruteforceResult.Found {
		details := generateAuthIssueDetails(bruteforceResult, historyItem)
		noTaskJob := uint(0)

		var issueCode db.IssueCode
		switch authType {
		case AuthTypeBasic:
			issueCode = db.WeakBasicAuthCredentialsCode
		case AuthTypeDigest:
			issueCode = db.WeakDigestAuthCredentialsCode
		default:
			issueCode = db.WeakAuthCredentialsCode
		}

		issue, err := db.CreateIssueFromHistoryAndTemplate(
			historyItem,
			issueCode,
			details,
			90,
			db.High.String(),
			historyItem.WorkspaceID,
			historyItem.TaskID,
			&noTaskJob,
		)

		if err != nil {
			log.Error().Err(err).
				Str("username", bruteforceResult.Username).
				Str("password", bruteforceResult.Password).
				Str("auth_type", string(authType)).
				Msg("Failed to create issue for weak authentication credentials")
			result.Error = fmt.Errorf("failed to create issue: %w", err)
			return result, result.Error
		}

		result.Issue = &issue
		log.Info().
			Str("username", bruteforceResult.Username).
			Str("password", bruteforceResult.Password).
			Str("auth_type", string(authType)).
			Uint("issue_id", issue.ID).
			Msg("Created issue for weak authentication credentials")
	}

	return result, nil
}

func generateAuthIssueDetails(result *AuthBruteforceResult, historyItem *db.History) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Weak authentication credentials were discovered through brute force attack on a %s %s endpoint at %s\n\n",
		string(result.AuthType), historyItem.Method, historyItem.URL))
	sb.WriteString(fmt.Sprintf("The credentials were successfully cracked in %.2f seconds after %d attempts.\n\n",
		result.Duration.Seconds(), result.Attempts))

	sb.WriteString("Discovered Credentials:\n")
	sb.WriteString(fmt.Sprintf("- Username: %s\n", result.Username))
	sb.WriteString(fmt.Sprintf("- Password: %s\n\n", result.Password))

	return sb.String()
}
