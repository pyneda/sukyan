package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// DockerAPIPaths contains core Docker API endpoints and a few version-specific paths
var DockerAPIPaths = []string{
	// Core API endpoints (version-agnostic)
	"_ping",
	"version",
	"info",
	"events",
	"system/info",
	"system/df",
	"containers/json",
	"images/json",
	"volumes",
	"networks",
	"services",
	"tasks",
	"swarm",
	"plugins",
	"nodes",

	// Most common configuration endpoints
	"auth",
	"build",
	"configs",
	"secrets",

	// A few specific version checks (most common)
	"v1.41/info",
	"v1.24/info",

	// Docker socket and environment checks
	"docker.sock",
	".dockerenv",

	// Registry endpoints
	"v2/_catalog",
	"v2/tags/list",
}

func IsDockerAPIValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	body, _ := history.ResponseBody()
	bodyStr := string(body)
	details := fmt.Sprintf("Docker API endpoint found: %s\n", history.URL)
	confidence := 0

	headers, _ := history.GetResponseHeadersAsMap()
	for header, values := range headers {
		headerStr := strings.ToLower(strings.Join(values, " "))
		if strings.Contains(headerStr, "docker") {
			confidence += 20
			details += fmt.Sprintf("- Docker-related header found: %s\n", header)
		}
	}

	switch history.StatusCode {
	case 200:
		// Docker API returns JSON, not HTML. HTML responses with words like
		// "container" or "image" are web apps, not Docker APIs.
		if strings.Contains(history.ResponseContentType, "text/html") {
			return false, "", 0
		}
		confidence += 20
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(bodyStr), &jsonData); err == nil {
			confidence += 30
			details += "- Valid JSON response received\n"
		}
	case 401, 403:
		confidence += 30
		details += "- Authentication required/forbidden (typical for protected Docker API)\n"
	case 404:
		// Only consider 404s if we have other strong indicators
		if confidence > 30 {
			details += "- Endpoint not found but Docker-related headers present\n"
		}
	default:
		confidence += 10
	}

	// Docker-specific patterns (case-sensitive, unlikely to appear in generic web content)
	dockerSpecificPatterns := map[string]string{
		"DockerRootDir":      "Docker root directory",
		"ServerVersion":      "Docker version",
		"OperatingSystem":    "system information",
		"ExperimentalBuild":  "build configuration",
		"LiveRestoreEnabled": "restore configuration",
		"docker daemon":      "daemon reference",
		"MemTotal":           "memory information",
		"NCPU":               "CPU information",
	}

	// Generic patterns that only count on JSON responses
	genericPatterns := map[string]string{
		"Containers": "container information",
		"Images":     "image information",
		"ApiVersion": "API version",
		"Registry":   "registry configuration",
		"Swarm":      "swarm information",
	}

	patternMatches := 0
	for pattern, description := range dockerSpecificPatterns {
		if strings.Contains(bodyStr, pattern) {
			patternMatches++
			details += fmt.Sprintf("- Contains %s\n", description)
		}
	}

	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 10
		details += "- Proper JSON content type\n"
		for pattern, description := range genericPatterns {
			if strings.Contains(bodyStr, pattern) {
				patternMatches++
				details += fmt.Sprintf("- Contains %s\n", description)
			}
		}
	}

	if patternMatches > 0 {
		confidence += min(patternMatches*10, 40)
	}

	// Check for obvious Docker-related content
	if strings.Contains(strings.ToLower(bodyStr), "docker") {
		confidence += 15
		details += "- Contains explicit Docker reference\n"
	}

	// Final confidence adjustment and return
	return confidence >= minConfidence(), details, min(confidence, 100)

}

func DiscoverDockerAPIEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       DockerAPIPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsDockerAPIValidationFunc,
		IssueCode:      db.DockerApiDetectedCode,
	})
}
