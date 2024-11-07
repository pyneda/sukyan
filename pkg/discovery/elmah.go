package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var ElmahPaths = []string{
	"elmah.axd",
}

func IsElmahValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("ASP.NET ELMAH handler detected at: %s\n", history.URL)
		confidence := 50

		contentType := strings.ToLower(history.ResponseContentType)
		if strings.Contains(contentType, "text/html") {
			confidence += 20
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

		return true, details, min(confidence, 100)
	}

	return false, "", 0
}

func DiscoverElmah(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       ElmahPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,*/*",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsElmahValidationFunc,
		IssueCode:      db.ElmahExposedCode,
	})
}
