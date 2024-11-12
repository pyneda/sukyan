package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var LogFilesPaths = []string{
	// Common log files
	"logs/",
	"log/",
	"error.log",
	"errors.log",
	"debug.log",
	"access.log",
	"access_log",
	"server.log",
	"app.log",
	"application.log",
	"audit.log",

	// Framework-specific logs
	"wp-content/debug.log",
	"laravel.log",
	"storage/logs/laravel.log",
	"var/log/",
	"var/logs/",
	"logger.php",

	// Development logs
	"npm-debug.log",
	"yarn-debug.log",
	"yarn-error.log",
	"php_error.log",
	"php-errors.log",

	// Server logs
	"apache/logs/",
	"apache2/logs/",
	"nginx/logs/",
	".pm2/logs/",
	"supervisor/logs/",

	// Application server logs
	"tomcat/logs/",
	"catalina.out",
	"jboss/server/log/",
	"weblogic/logs/",
}

func IsLogFileValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Log file found: %s\n", history.URL)
	confidence := 0

	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "application/octet-stream") {
		confidence += 20
		details += "- Appropriate content type for log file\n"
	}

	logPatterns := []string{
		"error",
		"warning",
		"notice",
		"info",
		"debug",
		"exception",
		"stack trace",
		"fatal",
		"failed",
		"timestamp",
		"[ERROR]",
		"[WARN]",
		"[INFO]",
		"[DEBUG]",
		"[NOTICE]",
		"Exception in thread",
		"Traceback",
		"stacktrace:",
	}

	matchCount := 0
	for _, pattern := range logPatterns {
		if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(pattern)) {
			matchCount++
		}
	}

	if matchCount > 0 {
		confidence += matchCount * 10
		details += fmt.Sprintf("- Contains %d common log patterns\n", matchCount)
	}

	timePatterns := []string{
		`\d{4}-\d{2}-\d{2}`,                     // YYYY-MM-DD
		`\d{2}:\d{2}:\d{2}`,                     // HH:MM:SS
		`\[\d{2}/\w{3}/\d{4}:\d{2}:\d{2}:\d{2}`, // Apache log format
	}

	for _, pattern := range timePatterns {
		if strings.Contains(bodyStr, pattern) {
			confidence += 15
			details += "- Contains timestamp patterns typical of log files\n"
			break
		}
	}

	if strings.Contains(bodyStr, "Stack trace:") ||
		strings.Contains(bodyStr, "PHP Fatal error:") ||
		strings.Contains(bodyStr, "PHP Warning:") {
		confidence += 20
		details += "- Contains PHP error log patterns\n"
	}

	if confidence >= 50 {
		if confidence > 100 {
			confidence = 100
		}
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverLogFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       LogFilesPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/plain,text/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsLogFileValidationFunc,
		IssueCode:      db.ExposedLogFileCode,
	})
}
