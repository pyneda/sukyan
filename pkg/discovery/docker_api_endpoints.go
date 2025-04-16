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

func IsDockerAPIValidationFunc(history *db.History) (bool, string, int) {
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

	dockerPatterns := map[string]string{
		// Core system information
		"OperatingSystem": "system information",
		"DockerRootDir":   "Docker root directory",
		"ServerVersion":   "Docker version",
		"ApiVersion":      "API version",

		// Resource information
		"Containers": "container information",
		"Images":     "image information",
		"MemTotal":   "memory information",
		"NCPU":       "CPU information",

		// Features and configuration
		"ExperimentalBuild":  "build configuration",
		"LiveRestoreEnabled": "restore configuration",
		"Registry":           "registry configuration",
		"Swarm":              "swarm information",

		// Common error messages that confirm Docker
		"docker daemon": "daemon reference",
		"container":     "container reference",
		"image":         "image reference",
	}

	patternMatches := 0
	for pattern, description := range dockerPatterns {
		if strings.Contains(bodyStr, pattern) {
			patternMatches++
			details += fmt.Sprintf("- Contains %s\n", description)
		}
	}

	// Adjust confidence based on pattern matches
	if patternMatches > 0 {
		confidence += min(patternMatches*10, 40)
	}

	if strings.Contains(history.ResponseContentType, "application/json") {
		confidence += 10
		details += "- Proper JSON content type\n"
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
