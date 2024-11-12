package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
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
		confidence += 30
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
			confidence += 30
			details += fmt.Sprintf("- Contains sensitive indicator: %s\n", description)
		}
	}

	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 30
		details += "- Correct content type for configuration files\n"
	}

	if strings.Contains(history.ResponseContentType, "text/yaml") ||
		strings.Contains(history.ResponseContentType, "application/x-yaml") {
		confidence += 30
		details += "- Correct content type for YAML configuration files\n"
	}

	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 30
		details += "- Correct content type for JSON configuration files\n"
	}

	if strings.Contains(history.ResponseContentType, "application/xml") ||
		strings.Contains(history.ResponseContentType, "text/xml") {
		confidence += 30
		details += "- Correct content type for XML configuration files\n"
	}

	if strings.Contains(history.ResponseContentType, "application/octet-stream") {
		confidence += 30
		details += "- Correct content type for binary configuration files\n"
	}

	return confidence >= minConfidence(), details, min(confidence, 100)

}

func DiscoverSensitiveConfigFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       ConfigFilePaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain,application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsSensitiveConfigFileValidationFunc,
		IssueCode:      db.SensitiveConfigDetectedCode,
	})
}
