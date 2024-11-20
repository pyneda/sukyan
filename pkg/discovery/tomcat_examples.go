package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var TomcatInfoLeakExamplePaths = []string{
	"/examples/jsp/snp/snoop.jsp",
	"/examples/servlet/RequestInfoExample",
	"/examples/servlet/RequestHeaderExample",
	"/examples/servlet/JndiServlet",
	"/examples/servlet/SessionExample",
	"/examples/jsp/sessions/carts.html",
	"/examples/servlet/CookieExample",
	"/examples/servlet/RequestParamExample",
	"/examples/jsp/include/include.jsp",
	"/examples/jsp/dates/date.jsp",
	"/examples/jsp/jsptoserv/jsptoservlet.jsp",
	"/examples/jsp/error/error.html",
	"/examples/jsp/forward/forward.jsp",
	"/examples/jsp/plugin/plugin.jsp",
	"/examples/jsp/mail/sendmail.jsp",
	"/examples/servlet/HelloWorldExample",
	"/examples/jsp/num/numguess.jsp",
	"/examples/jsp/checkbox/check.html",
	"/examples/jsp/colors/colors.html",
	"/examples/jsp/cal/login.html",
	"/examples/jsp/simpletag/foo.jsp",
	"/tomcat-docs/appdev/sample/web/hello.jsp",
}

func IsTomcatExampleValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := make([]string, 0)
	confidence := 0

	if strings.Contains(bodyStr, "Licensed to the Apache Software Foundation (ASF)") {
		confidence += 30
		details = append(details, "Contains Apache Software Foundation license header")
	}

	pathSpecificIndicators := map[string]struct {
		indicator   string
		description string
		value       int
	}{
		"snoop.jsp": {
			indicator:   "Request Information",
			description: "Exposes detailed request information including server details",
			value:       30,
		},
		"date.jsp": {
			indicator:   "Day of month: is",
			description: "Displays server time information",
			value:       25,
		},
		"plugin.jsp": {
			indicator:   "Current time is",
			description: "Contains Java plugin example",
			value:       20,
		},
		"error.html": {
			indicator:   "errorpage directive",
			description: "Error handling example page",
			value:       20,
		},
	}

	infoLeakIndicators := map[string]struct {
		value  int
		detail string
	}{
		"Request URI:":        {50, "Exposes request URI information"},
		"Server name:":        {50, "Exposes server name"},
		"Server port:":        {50, "Exposes server port number"},
		"Remote address:":     {50, "Exposes client IP addresses"},
		"Servlet path:":       {50, "Exposes servlet path information"},
		"Request Protocol:":   {50, "Exposes protocol information"},
		"Remote host:":        {50, "Exposes remote host information"},
		"JSP Request Method:": {50, "Exposes request method details"},
	}

	for pathKey, info := range pathSpecificIndicators {
		if strings.Contains(history.URL, pathKey) && strings.Contains(bodyStr, info.indicator) {
			confidence += info.value
			details = append(details, fmt.Sprintf("Identified %s example: %s", pathKey, info.description))
		}
	}

	for indicator, info := range infoLeakIndicators {
		if strings.Contains(bodyStr, indicator) {
			confidence += info.value
			details = append(details, info.detail)
		}
	}

	if headers, err := history.GetResponseHeadersAsMap(); err == nil {
		if server, exists := headers["Server"]; exists {
			for _, value := range server {
				if strings.Contains(value, "Apache-Coyote") {
					confidence += 20
					details = append(details, "Apache Tomcat server signature detected")
					break
				}
			}
		}
	}

	return confidence >= minConfidence(), strings.Join(details, "\n"), min(confidence, 100)

}

func DiscoverTomcatExamples(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       TomcatInfoLeakExamplePaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsTomcatExampleValidationFunc,
		IssueCode:      db.TomcatExamplesInfoLeakCode,
	})
}
