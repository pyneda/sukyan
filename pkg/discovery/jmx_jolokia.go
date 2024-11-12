package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var JMXHttpPaths = []string{
	// Jolokia paths (HTTP JMX Bridge)
	"jolokia",
	"jolokia/",
	"api/jolokia",
	"monitoring/jolokia",
	"actuator/jolokia",
	"management/jolokia",
	"jolokia/list",
	"jolokia/version",
	"jolokia/search/*",

	// Spring Boot Actuator JMX exposure
	"actuator/jmx",
	"management/jmx",

	// Tomcat Manager JMX Proxy
	"manager/jmxproxy",
	"manager/status/all",

	// Older/Legacy HTTP JMX interfaces
	"jmx-console/HtmlAdaptor",
	"web-console/Invoker/JMX",
	"system/console/jmx",
	"admin/jmx",
}

func IsHTTPJMXValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("HTTP JMX endpoint found: %s\n", history.URL)
	confidence := 0

	// Jolokia specific patterns (most common HTTP JMX bridge)
	jolokiaPatterns := []struct {
		pattern     string
		description string
		weight      int
	}{
		{`"agent"`, "Jolokia agent info", 25},
		{`"protocol"`, "Jolokia protocol info", 20},
		{`"config"`, "Jolokia config", 20},
		{`"value"`, "JMX value response", 15},
		{`"timestamp"`, "Jolokia timestamp", 15},
		{`"status"`, "Jolokia status", 15},
		{`"type":"read"`, "Jolokia read operation", 20},
		{`"type":"list"`, "Jolokia list operation", 20},
		{`"type":"version"`, "Jolokia version info", 25},
	}

	matches := make([]string, 0)
	for _, pattern := range jolokiaPatterns {
		if strings.Contains(bodyStr, pattern.pattern) {
			confidence += pattern.weight
			matches = append(matches, pattern.description)
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	for key := range headers {
		if strings.ToLower(key) == "x-jolokia-agent" {
			confidence += 30
			matches = append(matches, "Jolokia agent header found")
		}
	}

	contentType := strings.ToLower(history.ResponseContentType)
	if strings.Contains(contentType, "application/json") {
		confidence += 10
		if strings.Contains(contentType, "jolokia") {
			confidence += 20
			matches = append(matches, "Jolokia content type")
		}
	}

	if len(matches) > 0 {
		details += "Found HTTP JMX-related patterns:\n"
		for _, match := range matches {
			details += fmt.Sprintf("- %s\n", match)
		}
	}

	if confidence >= 50 {
		if confidence > 100 {
			confidence = 100
		}
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverHTTPJMXEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       JMXHttpPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "application/json,text/html",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsHTTPJMXValidationFunc,
		IssueCode:      db.ExposedJolokiaEndpointCode,
	})
}
