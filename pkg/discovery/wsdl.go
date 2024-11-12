package discovery

import (
	"encoding/xml"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

// WSDLPaths contains common paths where WSDL files might be found
var WSDLPaths = []string{
	// Generic WSDL paths
	"service?wsdl",
	"service.wsdl",
	"services/service.wsdl",
	"wsdl",
	"?wsdl",

	// Java/Spring framework paths
	"ws/service?wsdl",
	"services?wsdl",
	"webservices?wsdl",
	"axis/services?wsdl",
	"axis2/services?wsdl",

	// .NET paths
	"Service.asmx?WSDL",
	"Service.asmx?wsdl",
	"webservice.asmx?wsdl",
	"service.svc?wsdl",
	"*.svc?wsdl",

	// PHP paths
	"soap?wsdl",
	"soap/service?wsdl",
	"soap/server?wsdl",
	"nusoap/service?wsdl",

	// Version-specific paths
	"v1/service?wsdl",
	"v2/service?wsdl",
	"api/v1/service?wsdl",
	"api/v2/service?wsdl",

	// Common framework-specific paths
	"axis2-web/services?wsdl",
	"jaxws/services?wsdl",
	"cxf/services?wsdl",
	"metro/services?wsdl",
}

// WSDL UI and service listing page markers
var wsdlUIMarkers = []string{
	"<title>web service",
	"<title>wsdl",
	"<title>soap service",
	"web service list",
	"available services",
	"web service tester",
	"axis2 services",
	"axis services",
	"cxf services",
	"metro services",
	"service description",
	"wsdl viewer",
	"soap ui",
}

func IsWSDLValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 404 {
		return false, "", 0
	}
	confidence := 50
	details := make([]string, 0)

	if history.StatusCode != 200 {
		confidence = 0
	}

	contentType := strings.ToLower(history.ResponseContentType)
	// Check for WSDL-specific content types
	if strings.Contains(contentType, "application/wsdl+xml") ||
		strings.Contains(contentType, "text/xml") {
		confidence += 15
		details = append(details, "WSDL/XML content type detected")
	}

	if isWSDLUI(history) {
		return true, "WSDL/Web Service UI detected", 95
	}

	bodyStr := string(history.ResponseBody)
	bodyLower := strings.ToLower(bodyStr)

	// Common WSDL XML elements and attributes
	commonFields := []string{
		"<wsdl:",
		"<soap:",
		"xmlns:wsdl=",
		"xmlns:soap=",
		"<definitions",
		"<types>",
		"<message>",
		"<portType>",
		"<binding>",
		"<service>",
		"<operation>",
		"<documentation>",
		"schemaLocation",
		"targetNamespace",
		"soapAction",
		"<input>",
		"<output>",
		"<fault>",
	}

	fieldCount := 0
	matchedFields := []string{}

	for _, field := range commonFields {
		if strings.Contains(bodyLower, strings.ToLower(field)) {
			fieldCount++
			matchedFields = append(matchedFields, field)
		}
	}

	// Increment confidence based on matches
	confidenceIncrement := 15
	confidence = min(confidence+(confidenceIncrement*fieldCount), 100)

	if fieldCount >= 2 {
		details = append(details, "Multiple WSDL elements detected:\n - "+strings.Join(matchedFields, "\n - "))

	}

	// Try to parse as XML and verify if it's a WSDL document
	type WSDLDefinitions struct {
		XMLName xml.Name `xml:"definitions"`
	}

	var wsdl WSDLDefinitions
	if err := xml.Unmarshal(history.ResponseBody, &wsdl); err == nil {
		if wsdl.XMLName.Local == "definitions" {
			confidence += 20
			details = append(details, "Valid WSDL XML structure detected")
		}
	}

	// Check headers for WSDL-related information
	headersStr, err := history.GetResponseHeadersAsString()
	if err == nil {
		headersLower := strings.ToLower(headersStr)
		if strings.Contains(headersLower, "wsdl") ||
			strings.Contains(headersLower, "soap") {
			confidence = min(confidence+20, 100)
			details = append(details, "Web service related header detected")
		}
	}

	if confidence > 50 {
		if confidence > 100 {
			confidence = 100
		}
		return true, strings.Join(details, "\n"), confidence
	}

	return false, "", 0
}

func isWSDLUI(history *db.History) bool {
	bodyStr := string(history.ResponseBody)
	bodyLower := strings.ToLower(bodyStr)

	if !strings.Contains(strings.ToLower(history.ResponseContentType), "text/html") {
		return false
	}

	markerCount := 0
	for _, marker := range wsdlUIMarkers {
		if strings.Contains(bodyLower, marker) {
			markerCount++
		}
	}

	return markerCount >= 2
}

func DiscoverWSDLDefinitions(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       WSDLPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "application/wsdl+xml, text/xml, application/xml, */*",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsWSDLValidationFunc,
		IssueCode:      db.WsdlDefinitionDetectedCode,
	})
}
