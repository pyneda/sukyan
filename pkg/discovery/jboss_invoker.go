package discovery

import (
	"strings"

	"github.com/pyneda/sukyan/db"
)

var JBossInvokerPaths = []string{
	"invoker/JMXInvokerServlet",
	"invoker/EJBInvokerServlet",
	"web-console/Invoker",
	"invoker/readonly",
}

func IsJBossInvokerValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode == 404 {
		return false, "", 0
	}

	confidence := 0
	details := make([]string, 0)

	body, _ := history.ResponseBody()
	bodyStr := string(body)
	if history.StatusCode == 500 {
		confidence += 40
		details = append(details, "JBoss invoker endpoint returned server error")
	}

	if strings.Contains(bodyStr, "java.lang.ClassNotFoundException") ||
		strings.Contains(bodyStr, "java.io.IOException") ||
		strings.Contains(bodyStr, "org.jboss") {
		confidence += 30
		details = append(details, "JBoss exception detected in response")
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

	if strings.Contains(bodyStr, "\xAC\xED\x00\x05") {
		confidence += 30
		details = append(details, "Java serialized data marker detected")
	}

	return confidence >= minConfidence(), strings.Join(details, "\n"), min(confidence, 100)
}

func DiscoverJBossInvokers(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       JBossInvokerPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "*/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsJBossInvokerValidationFunc,
		IssueCode:      db.JbossInvokerDetectedCode,
	})
}
