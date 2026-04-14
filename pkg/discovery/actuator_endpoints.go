package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var ActuatorPaths = []string{
	"actuator",
	"actuator/env",
	"actuator/health",
	"actuator/beans",
	"actuator/conditions",
	"actuator/configprops",
	"actuator/env",
	"actuator/flyway",
	"actuator/httptrace",
	"actuator/integrationgraph",
	"actuator/liquibase",
	"actuator/loggers",
	"actuator/metrics",
	"actuator/mappings",
	"actuator/scheduledtasks",
	"actuator/sessions",
	"actuator/shutdown",
	"actuator/threaddump",
	"actuator/heapdump",
	"manage",
	"manage/env",
	"manage/health",
	"manage/beans",
}

func IsActuatorValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	confidence := 40
	var details string
	body, _ := history.ResponseBody()
	bodyStr := string(body)

	if history.StatusCode == 200 {
		details = fmt.Sprintf("Spring Boot Actuator endpoint found: %s\n", history.URL)

		var jsonBody map[string]interface{}
		if strings.Contains(history.ResponseContentType, "application/json") {
			confidence += 10
			if err := json.Unmarshal(body, &jsonBody); err == nil {

				// Spring-specific keys that distinguish actuator from generic JSON APIs
				springKeys := map[string]struct {
					weight int
					detail string
				}{
					"propertySources": {40, "Exposes environment properties"},
					"beans":           {40, "Exposes application beans configuration"},
					"contexts":        {35, "Exposes application contexts"},
					"configprops":     {35, "Exposes configuration properties"},
					"_links":          {30, "HAL-style actuator index"},
				}

				springKeyFound := false
				for key, info := range springKeys {
					if _, exists := jsonBody[key]; exists {
						confidence += info.weight
						details += fmt.Sprintf("- %s\n", info.detail)
						springKeyFound = true
					}
				}

				// "status" alone is too generic (matches status pages, health checks, etc.)
				// Only count it if we also found a Spring-specific key
				if _, exists := jsonBody["status"]; exists && springKeyFound {
					confidence += 20
					details += "- Exposes application status information\n"
				}

				sensitivePatterns := []string{"spring.datasource", "spring.mail", "secret", "password", "token", "aws", "azure"}
				for _, pattern := range sensitivePatterns {
					if strings.Contains(strings.ToLower(bodyStr), pattern) {
						details += fmt.Sprintf("- Contains potentially sensitive information related to: %s\n", pattern)
						confidence += 10
					}
				}
			}
		}

	}

	return confidence >= minConfidence(), details, min(confidence, 100)
}

func DiscoverActuatorEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       ActuatorPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "application/json",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsActuatorValidationFunc,
		IssueCode:      db.ExposedSpringActuatorEndpointsCode,
	})
}
