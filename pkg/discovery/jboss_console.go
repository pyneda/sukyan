package discovery

import (
	"strings"

	"github.com/pyneda/sukyan/db"
)

var JBossConsolePaths = []string{
	"jmx-console/HtmlAdaptor",
	"web-console/ServerInfo.jsp",
	"admin-console",
	"web-console",
	"jmx-console",
}

func IsJBossConsoleValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 404 {
		return false, "", 0
	}

	body, _ := history.ResponseBody()
	bodyStr := string(body)
	details := make([]string, 0)
	confidence := 0

	consoleIndicators := map[string]struct {
		description string
		weight      int
	}{
		"JBoss Management Console": {description: "JBoss management console title", weight: 40},
		"JMX Agent View":           {description: "JMX console interface", weight: 40},
		"JBoss Administration":     {description: "Admin interface detected", weight: 35},
		"Welcome to JBoss":         {description: "JBoss welcome page", weight: 30},
		"HtmlAdaptor":              {description: "JMX HTML adapter", weight: 35},
		"ServerInfo":               {description: "Server information page", weight: 30},
		"jboss.system:type=Server": {description: "JBoss system information", weight: 40},
	}

	for indicator, info := range consoleIndicators {
		if strings.Contains(bodyStr, indicator) {
			confidence += info.weight
			details = append(details, info.description)
		}
	}

	if history.StatusCode == 401 || history.StatusCode == 403 {
		confidence += 30
		details = append(details, "Protected console interface")
	}

	if history.StatusCode == 200 {
		confidence += 40
		details = append(details, "Publicly accessible console interface")
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

func DiscoverJBossConsoles(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       JBossConsolePaths,
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
		ValidationFunc: IsJBossConsoleValidationFunc,
		IssueCode:      db.JbossConsoleDetectedCode,
	})
}
