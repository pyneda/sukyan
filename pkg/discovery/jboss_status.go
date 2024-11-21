package discovery

import (
	"strings"

	"github.com/pyneda/sukyan/db"
)

var JBossStatusPaths = []string{
	"status",
	"status?full=true",
	"web-console/status",
	"jboss-status",
	"server-status",
	"system/status",
}

func IsJBossStatusValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 404 {
		return false, "", 0
	}

	confidence := 0
	details := make([]string, 0)
	bodyStr := string(history.ResponseBody)

	statusIndicators := map[string]struct {
		pattern     string
		description string
		weight      int
	}{
		"status": {
			pattern:     "<title>Tomcat Status",
			description: "Tomcat status page",
			weight:      50,
		},
		"memoryInfo": {
			pattern:     "Memory:",
			description: "Memory information exposed",
			weight:      50,
		},
		"jvmInfo": {
			pattern:     "JVM Version",
			description: "JVM version information exposed",
			weight:      50,
		},
		"osInfo": {
			pattern:     "OS Name",
			description: "Operating system information exposed",
			weight:      50,
		},
		"threadInfo": {
			pattern:     "ThreadPool",
			description: "Thread pool information exposed",
			weight:      50,
		},
		"systemProperties": {
			pattern:     "System Properties",
			description: "System properties exposed",
			weight:      55,
		},
	}

	for _, info := range statusIndicators {
		if strings.Contains(bodyStr, info.pattern) {
			confidence += info.weight
			details = append(details, info.description)
		}
	}

	if history.StatusCode == 200 {
		confidence += 20
		details = append(details, "Status page publicly accessible")
	}

	if history.StatusCode == 401 || history.StatusCode == 403 {
		confidence += 30
		details = append(details, "Protected status page")
	}

	headers, _ := history.GetResponseHeadersAsMap()
	for _, values := range headers {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), "jboss") {
				confidence += 20
				details = append(details, "JBoss server signature in headers")
				break
			}
		}
	}

	return confidence >= minConfidence(), strings.Join(details, "\n"), min(confidence, 100)
}

func DiscoverJBossStatus(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       JBossStatusPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsJBossStatusValidationFunc,
		IssueCode:      db.JbossStatusDetectedCode,
	})
}
