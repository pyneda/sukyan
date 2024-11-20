package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var EnvFilePaths = []string{
	".env",
	".env.local",
	".env.backup",
	".env.dev",
	".env.development",
	".env.prod",
	".env.production",
	".env.example",
}

func IsEnvFileValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("Environment configuration file detected at: %s\n", history.URL)
		confidence := 50

		contentType := strings.ToLower(history.ResponseContentType)
		if strings.Contains(contentType, "text/plain") {
			confidence += 20
		}

		envPatterns := map[string]string{
			"DB_HOST":  "Database connection",
			"API_KEY":  "API credentials",
			"SECRET":   "Secret keys",
			"PASSWORD": "Password configuration",
			"AWS_":     "AWS configuration",
			"STRIPE_":  "Stripe integration",
			"MAIL_":    "Mail configuration",
			"APP_":     "Application configuration",
			"REDIS_":   "Redis configuration",
			"TOKEN":    "Authentication tokens",
			"_URL=":    "Service URLs",
			"PORT=":    "Service ports",
			"DEBUG":    "Debugging configuration",
		}

		foundPatterns := make([]string, 0)
		for pattern, description := range envPatterns {
			if strings.Contains(bodyStr, pattern) {
				confidence += 30
				foundPatterns = append(foundPatterns, description)
			}
		}

		if len(foundPatterns) == 0 && strings.Contains(contentType, "text/html") {
			return false, "", 0
		} else if strings.Contains(contentType, "text/html") {
			confidence -= 50
		} else {
			// Check for KEY=VALUE pattern which is typical in env files
			lines := strings.Split(bodyStr, "\n")
			envLineCount := 0
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") && strings.Contains(line, "=") {
					envLineCount++
				}
			}

			if envLineCount > 0 {
				confidence += 30
				details += fmt.Sprintf("- Contains %d environment variable assignments\n", envLineCount)
			}

		}

		if len(foundPatterns) > 0 {
			details += "\nDetected configuration types:\n"
			for _, pattern := range foundPatterns {
				details += fmt.Sprintf("- %s\n", pattern)
			}
		}

		if confidence > 30 {
			return true, details, min(confidence, 100)
		}
	}

	return false, "", 0
}

func DiscoverEnvFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       EnvFilePaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/plain,*/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsEnvFileValidationFunc,
		IssueCode:      db.EnvironmentFileExposedCode,
	})
}
