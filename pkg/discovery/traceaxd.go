package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var AspNetTracePaths = []string{
	"trace.axd",
}

// IsAspNetTraceValidationFunc validates if the response indicates an exposed ASP.NET trace page
func IsAspNetTraceValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		bodyStr := string(history.ResponseBody)
		details := fmt.Sprintf("ASP.NET Trace Viewer detected at: %s\n", history.URL)
		confidence := 40

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
			details += "\nWARNING: ASP.NET tracing is enabled and publicly accessible. This can expose sensitive application details including:\n"
			details += "- Application variables and session state\n"
			details += "- Request/response headers and cookies\n"
			details += "- Server variables and configuration\n"
			details += "- Detailed error messages and stack traces\n"
			return true, details, min(confidence, 100)
		}
	}

	return false, "", 0
}

func DiscoverAspNetTrace(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       AspNetTracePaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsAspNetTraceValidationFunc,
		IssueCode:      db.AspnetTraceEnabledCode,
	})
}
