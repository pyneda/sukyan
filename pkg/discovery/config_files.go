package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var ConfigFilePaths = []string{
	".env",
	"config.php",
	"settings.php",
	"config.yaml",
	"application.yml",
	"appsettings.json",
	"web.config",
	"config.json",
	"database.yml",
	"local.settings.json",
	"secrets.json",
	"parameters.yml",
	"private.key",
	"jwt.key",
	"deploy.rsa",
	"deployment.yaml",
	"kubeconfig",
	"docker-compose.yml",
	"nginx.conf",
	"httpd.conf",
}

func IsSensitiveConfigFileValidationFunc(history *db.History) (bool, string, int) {
	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Potential sensitive configuration file detected: %s\n", history.URL)
	confidence := 0

	// Base confidence for 200 status code response
	if history.StatusCode == 200 {
		confidence += 50
		details += "- Received 200 OK status for configuration file endpoint\n"
	}

	// Additional sensitive data indicators in the body
	configIndicators := map[string]string{
		"DB_PASSWORD":       "Database password",
		"DB_USERNAME":       "Database username",
		"API_KEY":           "API key",
		"secret_key":        "Secret key",
		"access_key":        "Access key",
		"AWS_SECRET_ACCESS": "AWS secret access key",
		"PRIVATE_KEY":       "Private key",
		"CLIENT_SECRET":     "OAuth client secret",
		"JWT_SECRET":        "JWT secret",
		"AUTH_TOKEN":        "Authentication token",
		"ENCRYPTION_KEY":    "Encryption key",
		"PASSWORD":          "Password field",
		"SENTRY_DSN":        "Sentry DSN (error logging)",
		"GOOGLE_API_KEY":    "Google API key",
		"SMTP_PASSWORD":     "SMTP server password",
	}

	for indicator, description := range configIndicators {
		if strings.Contains(bodyStr, indicator) {
			confidence += 10
			details += fmt.Sprintf("- Contains sensitive indicator: %s\n", description)
		}
	}

	// Check for sensitive headers (e.g., exposing content-type as text/plain)
	headers, err := history.GetResponseHeadersAsMap()
	if err == nil {
		if contentType, ok := headers["Content-Type"]; ok {
			for _, val := range contentType {
				if strings.Contains(val, "text/plain") {
					confidence += 10
					details += "- Content-Type is text/plain, possibly exposing raw data\n"
					break
				}
			}
		}
	}

	// Return true if confidence is at a reasonable level
	if confidence >= 50 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverSensitiveConfigFiles(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       ConfigFilePaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsSensitiveConfigFileValidationFunc,
		IssueCode:      db.SensitiveConfigDetectedCode,
	})
}
