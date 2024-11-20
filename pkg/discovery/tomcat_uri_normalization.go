package discovery

import (
	"strings"

	"github.com/pyneda/sukyan/db"
)

var TomcatUriNormalizationPatterns = []string{
	"..;/manager/html",
	"..;/",
	"%2e%2e%3b/manager/html",
	"%252E%252E/manager/html",
}

func IsTomcatManagerResponse(history *db.History) (bool, string, int) {
	confidence := 0
	details := make([]string, 0)

	if history.StatusCode == 401 {
		headers, err := history.GetResponseHeadersAsMap()
		if err == nil {
			for headerName, headerValues := range headers {
				if strings.ToLower(headerName) == "www-authenticate" {
					confidence += 50
					for _, value := range headerValues {
						if strings.Contains(strings.ToLower(value), "tomcat manager") {
							confidence = 90
							details = append(details, "Found Tomcat Manager authentication header")
							break
						}
					}
				}
			}
		}
	}

	if history.StatusCode == 200 {
		responseBody := string(history.ResponseBody)
		confidence += 10
		if strings.Contains(responseBody, "Apache Tomcat") {
			confidence += 50
			details = append(details, "Found Apache Tomcat signature in response")
		}
	}

	return confidence >= minConfidence(), strings.Join(details, "\n"), min(confidence, 100)
}

func DiscoverTomcatUriNormalization(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       TomcatUriNormalizationPatterns,
			Concurrency: DefaultTimeout,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsTomcatManagerResponse,
		IssueCode:      db.TomcatUriNormalizationCode,
	})
}
