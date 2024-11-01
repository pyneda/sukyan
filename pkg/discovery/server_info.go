package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
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
			confidence += 20
			details += fmt.Sprintf("- Contains server information: %s\n", indicator)
		}
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 40 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverServerInfo(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       ServerInfoPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,text/plain",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: isServerInfoValidationFunc,
		IssueCode:      db.ServerInfoDetectedCode,
	})
}
