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

func IsActuatorValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode == 200 {
		details := fmt.Sprintf("Spring Boot Actuator endpoint found: %s\n", history.URL)
		confidence := 40

		var jsonBody map[string]interface{}
		if strings.Contains(history.ResponseContentType, "application/json") {
			confidence += 10
			if err := json.Unmarshal([]byte(history.ResponseBody), &jsonBody); err == nil {
				bodyStr := string(history.ResponseBody)

				if _, exists := jsonBody["status"]; exists {
					confidence += 30
					details += "- Exposes application status information\n"
				}
				if _, exists := jsonBody["propertySources"]; exists {
					confidence += 40
					details += "- Exposes environment properties\n"
				}
				if _, exists := jsonBody["beans"]; exists {
					confidence += 40
					details += "- Exposes application beans configuration\n"
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

		return confidence >= 50, details, min(confidence, 100)
	}

	return false, "", 0
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
		},
		ValidationFunc: IsActuatorValidationFunc,
		IssueCode:      db.ExposedSpringActuatorEndpointsCode,
	})
}
