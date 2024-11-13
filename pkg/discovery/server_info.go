package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var ServerInfoPaths = []string{
	"server-info",
	"server-status",
	"status",
	".httpd/server-status",
	"apache/server-status",
	"apache2/server-status",
	"apache-status",
	"nginx_status",
	"nginx-status",
	"httpd/server-info",
	"apache2/server-info",
}

func isServerInfoValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyLower := strings.ToLower(string(history.ResponseBody))
	details := fmt.Sprintf("Server information page found: %s\n", history.URL)
	confidence := 20

	serverIndicators := []string{
		"uptime",
		"cpu load",
		"cpu usage",
		"memory usage",
		"total accesses",
		"requests per second",
		"bytes per second",
		"server version",
		"server built",
		"current time",
		"restart time",
		"active connections",
		"server statistics",
		"server status",
		"server configuration",
		"modules",
		"version",
	}

	for _, indicator := range serverIndicators {
		if strings.Contains(bodyLower, indicator) {
			if indicator == "version" || indicator == "modules" {
				confidence += 5
			} else {
				confidence += 30
			}
			details += fmt.Sprintf("- Contains server information: %s\n", indicator)
		}
	}

	return confidence >= minConfidence(), details, min(confidence, 100)

}

func DiscoverServerInfo(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       ServerInfoPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,text/plain",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: isServerInfoValidationFunc,
		IssueCode:      db.ServerInfoDetectedCode,
	})
}
