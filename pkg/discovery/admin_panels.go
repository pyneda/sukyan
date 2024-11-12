package discovery

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pyneda/sukyan/db"
)

var AdminPaths = []string{
	"admin", "administrator", "admin.php", "controlpanel", "dashboard",
	"manage", "wp-admin", "cpanel", "auth/admin", "secure/admin",
	"backend", "system/console", "admin-console",
}

func IsAdminInterfaceValidationFunc(history *db.History) (bool, string, int) {
	details := fmt.Sprintf("Potential admin interface detected: %s\n", history.URL)
	confidence := 0

	headers, err := history.GetResponseHeadersAsMap()
	if err != nil {
		return false, "", 0
	}

	if history.StatusCode == 200 {
		confidence = 50
		details += "- Received 200 OK status\n"

		adminKeywords := []struct {
			Keyword     string
			Description string
			Confidence  int
		}{
			{"admin", "Admin keyword in content", 10},
			{"dashboard", "Dashboard keyword in content", 10},
			{"control panel", "Control panel keyword in content", 10},
			{"manage account", "Manage account keyword in content", 10},
			{"administrator", "Administrator keyword in content", 10},
			{"admin login", "Admin login keyword in content", 10},
		}

		bodyContent := strings.ToLower(string(history.ResponseBody))
		for _, keyword := range adminKeywords {
			if strings.Contains(bodyContent, keyword.Keyword) {
				confidence += keyword.Confidence
				details += fmt.Sprintf("- Found %s\n", keyword.Description)
			}
		}

		if strings.Contains(history.ResponseContentType, "text/html") {
			confidence += 10
			details += "- Content-Type is HTML\n"

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(history.ResponseBody)))
			if err == nil {
				if doc.Find(`input[type="password"]`).Length() > 0 || doc.Find(`input[name="username"], input[name="password"]`).Length() > 0 {
					confidence += 20
					details += "- Contains login form elements\n"
				}
			}
		}
	} else if history.StatusCode == 401 || history.StatusCode == 403 {
		confidence = 80
		details += "- Received restricted access status (401 or 403)\n"
		if authHeader, ok := headers["WWW-Authenticate"]; ok {
			for _, val := range authHeader {
				if strings.Contains(strings.ToLower(val), "basic") || strings.Contains(strings.ToLower(val), "digest") {
					confidence += 10
					details += "- WWW-Authenticate header indicates authentication prompt\n"
					break
				}
			}
		}
	}

	return confidence >= 50, details, min(confidence, 100)
}

func DiscoverAdminInterfaces(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       AdminPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsAdminInterfaceValidationFunc,
		IssueCode:      db.AdminInterfaceDetectedCode,
	})
}
