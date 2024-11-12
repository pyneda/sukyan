package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var AspNetTracePaths = []string{
	"trace.axd",
}

// IsAspNetTraceValidationFunc validates if the response indicates an exposed ASP.NET trace page
func IsAspNetTraceValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("ASP.NET Trace Viewer detected at: %s\n", history.URL)
		confidence := 30

		if strings.Contains(strings.ToLower(history.ResponseContentType), "text/html") {
			confidence += 10
		}

		traceIndicators := map[string]string{
			"Application Trace":          "Application trace header",
			"Trace Information":          "Trace information section",
			"Request Details":            "Request details section",
			"Trace Version":              "Trace version information",
			"Show Details":               "Interactive trace viewer",
			"Session State":              "Session state information",
			"Application Variables":      "Application variables section",
			"Request Headers Collection": "Request headers collection",
		}

		for indicator, description := range traceIndicators {
			if strings.Contains(bodyStr, indicator) {
				confidence += 15
				details += fmt.Sprintf("- Contains %s\n", description)
			}
		}

		layoutIndicators := []string{
			"<table class=\"viewerDataTable",
			"<div class=\"traceInfo",
			"TraceHandler",
			"System.Web.TraceContext",
		}

		for _, indicator := range layoutIndicators {
			if strings.Contains(bodyStr, indicator) {
				confidence += 10
			}
		}

		if confidence >= 50 {
			return true, details, min(confidence, 100)
		}
	}

	return false, "", 0
}

func DiscoverAspNetTrace(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       AspNetTracePaths,
			Concurrency: DefaultConcurrency,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: IsAspNetTraceValidationFunc,
		IssueCode:      db.AspnetTraceEnabledCode,
	})
}
