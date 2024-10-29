package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var WordPressPaths = []string{
	"wp-login.php",
	"wp-admin/",
	"wp-content/",
	"wp-includes/",
	"wp-json/",
	"xmlrpc.php",
}

func IsWordPressValidationFunc(history *db.History) (bool, string, int) {
	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("WordPress installation detected: %s\n", history.URL)
	confidence := 0

	// Retrieve response headers as a map
	headers, err := history.GetResponseHeadersAsMap()
	if err != nil {
		return false, "", 0 // Return immediately if there's an error retrieving headers
	}

	// Define indicators with their descriptions and confidence levels
	wpIndicators := []struct {
		Indicator   string
		Description string
		Confidence  int
	}{
		{"wp-content/", "WordPress content directory", 20},
		{"wp-includes/", "WordPress includes directory", 20},
		{"<meta name=\"generator\" content=\"WordPress", "WordPress generator meta tag", 30},
		{"<link rel='https://api.w.org/'", "WordPress REST API link tag", 20},
		{"xmlrpc.php", "WordPress XML-RPC endpoint", 20},
	}

	// Check response body for indicators
	if history.StatusCode == 200 {
		for _, indicator := range wpIndicators {
			if strings.Contains(bodyStr, indicator.Indicator) {
				confidence += indicator.Confidence
				details += fmt.Sprintf("- Contains %s\n", indicator.Description)
			}
		}

		// Check for wp-json response
		if strings.Contains(history.URL, "wp-json/") && strings.Contains(history.ResponseContentType, "application/json") {
			if strings.Contains(bodyStr, "\"namespaces\"") && strings.Contains(bodyStr, "\"wp:") {
				confidence += 40
				details += "- WordPress REST API endpoint detected\n"
			}
		}
	}

	// Check specific headers for WordPress indicators
	if poweredBy, ok := headers["X-Powered-By"]; ok {
		for _, val := range poweredBy {
			if strings.Contains(val, "WordPress") {
				confidence += 20
				details += "- X-Powered-By header indicates WordPress\n"
				break
			}
		}
	}

	if pingback, ok := headers["X-Pingback"]; ok {
		for _, val := range pingback {
			if strings.Contains(val, "xmlrpc.php") {
				confidence += 20
				details += "- X-Pingback header indicates WordPress\n"
				break
			}
		}
	}

	// Return true if confidence indicates possible WordPress presence
	if confidence >= 50 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverWordPressEndpoints(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       WordPressPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsWordPressValidationFunc,
	})
}
