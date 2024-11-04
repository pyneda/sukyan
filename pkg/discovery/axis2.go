package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var Axis2Paths = []string{
	// Core admin interfaces
	"axis2-admin/",
	"axis2/axis2-admin/",
	"axis2-web/",
	"axis2/",

	// Service listing and management
	"axis2/services/",
	"axis2/services/listServices",
	"services/ListServices",
	"axis2-web/services/listServices",
	"axis2/axis2-web/services/listServices",

	// Status and verification pages
	"axis2-web/HappyAxis.jsp",
	"axis2/axis2-web/HappyAxis.jsp",
	"axis2-web/index.jsp",
	"axis2/axis2-web/index.jsp",

	// Admin functions
	"axis2-admin/listService",
	"axis2-admin/listFaultyServices",
	"axis2-admin/engagingglobally",
	"axis2-admin/selectService",

	// WSDL and service info
	"services/?wsdl",
	"axis2?wsdl",
	"axis2/services/?wsdl",
}

type axis2Pattern struct {
	pattern     string
	description string
	weight      int
	location    string // can be "body", "header", "url", or "error"
}

var axis2Fingerprints = []axis2Pattern{
	// HTML Title patterns
	{pattern: "<title>Axis 2 - ", description: "Axis2 page title", weight: 30, location: "body"},
	{pattern: "<title>Axis2 :: ", description: "Axis2 page title variation", weight: 30, location: "body"},
	{pattern: "<title>Welcome to Apache Axis2", description: "Axis2 welcome page", weight: 35, location: "body"},

	// Admin interface patterns
	{pattern: "Axis2 Administration Page", description: "Admin interface", weight: 40, location: "body"},
	{pattern: "Axis2 Happiness Page", description: "Test page", weight: 40, location: "body"},
	{pattern: "Axis2 Service Management", description: "Service management page", weight: 35, location: "body"},
	{pattern: "Welcome to Axis2 Web Admin Module", description: "Admin module", weight: 40, location: "body"},

	// Service listing patterns
	{pattern: "Available services in this axis2 instance", description: "Service listing", weight: 35, location: "body"},
	{pattern: "Available Services & Operations", description: "Service operations list", weight: 35, location: "body"},
	{pattern: "System Components", description: "System info page", weight: 30, location: "body"},
	{pattern: "Deployed Services", description: "Service deployment page", weight: 35, location: "body"},
	{pattern: "Service Groups", description: "Service groups listing", weight: 30, location: "body"},

	// Technical indicators
	{pattern: "org.apache.axis2", description: "Axis2 package reference", weight: 25, location: "error"},
	{pattern: "org.apache.axiom", description: "Axiom package reference", weight: 25, location: "error"},
	{pattern: "AxisServlet", description: "Axis2 servlet reference", weight: 30, location: "error"},
	{pattern: "Apache-Axis2", description: "Server identification", weight: 35, location: "header"},

	// WSDL related
	{pattern: "?wsdl", description: "WSDL endpoint", weight: 20, location: "url"},
	{pattern: "wsdl:definitions", description: "WSDL content", weight: 30, location: "body"},
	{pattern: "xmlns:axis2", description: "Axis2 XML namespace", weight: 25, location: "body"},

	// Form patterns
	{pattern: `<form name="uploadForm"`, description: "Service upload form", weight: 30, location: "body"},
	{pattern: `<form name="selectServiceForm"`, description: "Service selection form", weight: 30, location: "body"},

	// Error patterns
	{pattern: "AxisFault", description: "Axis2 fault", weight: 35, location: "error"},
	{pattern: "org.apache.axis2.AxisFault", description: "Detailed Axis2 fault", weight: 35, location: "error"},
	{pattern: "The Service cannot be found", description: "Service error", weight: 25, location: "error"},
	{pattern: "The requested service is not available", description: "Service error", weight: 25, location: "error"},

	// Configuration patterns
	{pattern: "engagingglobally", description: "Global configuration", weight: 25, location: "body"},
	{pattern: "engageToService", description: "Service configuration", weight: 25, location: "body"},
	{pattern: "View Global Chain", description: "Handler chain view", weight: 25, location: "body"},

	// Version related
	{pattern: "Apache Software Foundation", description: "Apache attribution", weight: 15, location: "body"},
	{pattern: "Apache Axis2 version:", description: "Version information", weight: 35, location: "body"},

	// Module related
	{pattern: "Available Modules", description: "Modules listing", weight: 30, location: "body"},
	{pattern: "Globally Engaged Modules", description: "Global modules", weight: 30, location: "body"},
	{pattern: "Module Administration", description: "Module admin", weight: 35, location: "body"},

	// Operation related
	{pattern: "Available Operations", description: "Operations listing", weight: 25, location: "body"},
	{pattern: "Operation Specific Parameters", description: "Operation params", weight: 25, location: "body"},

	// Authentication related
	{pattern: "username", description: "Authentication form", weight: 20, location: "body"},
	{pattern: "Invalid auth credentials", description: "Auth error", weight: 25, location: "error"},

	// XML patterns
	{pattern: "application/xml", description: "XML content type", weight: 15, location: "header"},
	{pattern: "text/xml", description: "XML content type", weight: 15, location: "header"},
}

func IsAxis2ValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 404 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Apache Axis2 endpoint found: %s\n", history.URL)
	confidence := 0
	matches := make([]string, 0)

	for _, fp := range axis2Fingerprints {
		switch fp.location {
		case "body":
			if strings.Contains(bodyStr, fp.pattern) {
				confidence += fp.weight
				matches = append(matches, fp.description)
			}
		case "error":
			if history.StatusCode >= 400 && strings.Contains(bodyStr, fp.pattern) {
				confidence += fp.weight
				matches = append(matches, "Error: "+fp.description)
			}
		case "url":
			if strings.Contains(history.URL, fp.pattern) {
				confidence += fp.weight
				matches = append(matches, "URL: "+fp.description)
			}
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	for key, values := range headers {
		headerKey := strings.ToLower(key)
		for _, value := range values {
			headerValue := strings.ToLower(value)

			for _, fp := range axis2Fingerprints {
				if fp.location == "header" {
					if (headerKey == "server" || headerKey == "x-powered-by") &&
						strings.Contains(headerValue, strings.ToLower(fp.pattern)) {
						confidence += fp.weight
						matches = append(matches, "Header: "+fp.description)
					}
				}
			}

			// Special case for content type headers
			if headerKey == "content-type" {
				if strings.Contains(headerValue, "application/wsdl+xml") {
					confidence += 20
					matches = append(matches, "WSDL content type detected")
				}
			}
		}
	}

	// Additional context-based confidence adjustments
	if strings.Contains(history.URL, "axis2-admin") || strings.Contains(history.URL, "axis2-web") {
		confidence += 10
	}

	if strings.Contains(bodyStr, "axis2") && strings.Contains(bodyStr, "apache") {
		confidence += 10
	}

	if len(matches) > 0 {
		details += "Found Axis2-related patterns:\n"
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

func DiscoverAxis2Endpoints(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       Axis2Paths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "text/html,application/xml,text/xml",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsAxis2ValidationFunc,
		IssueCode:      db.ExposedAxis2EndpointCode,
	})
}
