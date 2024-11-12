package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var WebServerControlPaths = []string{
	".htaccess",
	".htpasswd",
	"admin/.htaccess",
	".htaccess.bak",
	".htpasswd.bak",
}

// IsWebServerControlFileValidationFunc validates if the response indicates an exposed access control file
func IsWebServerControlFileValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("Web server access control file detected at: %s\n", history.URL)
		confidence := 30

		contentType := strings.ToLower(history.ResponseContentType)
		if strings.Contains(contentType, "text/plain") {
			confidence += 30
			details += "- Served with text/plain content type\n"
		}

		directives := map[string]string{
			"RewriteEngine": "URL rewriting configuration",
			"AuthType":      "Authentication configuration",
			"AuthUserFile":  "Authentication user file path",
			"Require":       "Access control directive",
			"Options":       "Directory options configuration",
			"Allow from":    "IP allowlist configuration",
			"Deny from":     "IP blocklist configuration",
			"php_value":     "PHP configuration",
			"<FilesMatch":   "File matching rules",
			":$apr1$":       "Password hash",
			":$2y$":         "Password hash",
		}

		foundDirectives := make([]string, 0)
		for directive, description := range directives {
			if strings.Contains(bodyStr, directive) {
				confidence += 25
				foundDirectives = append(foundDirectives, description)
			}
		}

		if len(foundDirectives) > 0 {
			details += "\nDetected configurations:\n"
			for _, directive := range foundDirectives {
				details += fmt.Sprintf("- %s\n", directive)
			}
		}

		return confidence >= minConfidence(), details, min(100, confidence)
	}

	return false, "", 0
}

// DiscoverWebServerControlFiles attempts to find exposed web server access control files
func DiscoverWebServerControlFiles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       WebServerControlPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/plain,text/html,*/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsWebServerControlFileValidationFunc,
		IssueCode:      db.WebserverControlFileExposedCode,
	})
}
