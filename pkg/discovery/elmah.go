package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var ElmahPaths = []string{
	"elmah.axd",
}

func IsElmahValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("ASP.NET ELMAH handler detected at: %s\n", history.URL)
		confidence := 30

		contentType := strings.ToLower(history.ResponseContentType)
		if strings.Contains(contentType, "text/html") {
			confidence += 10
		}

		elmahPatterns := map[string]string{
			"Error Log for":      "Log interface header",
			"Powered by ELMAH":   "ELMAH signature",
			"All Errors":         "Error listing",
			"errorDetail":        "Error details",
			"Download Log":       "Log download interface",
			"RSS Feed of Errors": "RSS feed",
			"Exception Details":  "Exception information",
			"Stack Trace":        "Stack traces",
			"Error Log Entry":    "Log entries",
			"Server Variables":   "Server data",
			"Form Variables":     "Form data",
			"Cookies Variables":  "Cookie data",
			"Application Errors": "Application errors",
		}

		foundPatterns := make([]string, 0)
		for pattern, description := range elmahPatterns {
			if strings.Contains(bodyStr, pattern) {
				confidence += 30
				foundPatterns = append(foundPatterns, description)
			}
		}

		if len(foundPatterns) > 0 {
			details += "\nDetected components:\n"
			for _, pattern := range foundPatterns {
				details += fmt.Sprintf("- %s\n", pattern)
			}
		}

		return confidence >= 50, details, min(confidence, 100)
	}

	return false, "", 0
}

func DiscoverElmah(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       ElmahPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,*/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsElmahValidationFunc,
		IssueCode:      db.ElmahExposedCode,
	})
}
