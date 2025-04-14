package generation

import (
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type LaunchConditionType string

const (
	Platform               LaunchConditionType = "platform"
	ScanMode               LaunchConditionType = "scan_mode"
	ParameterValueDataType LaunchConditionType = "parameter_value_data_type"
	ParameterName          LaunchConditionType = "insertion_point_name"
	ResponseCondition      LaunchConditionType = "response_condition"
)

type LaunchCondition struct {
	Type              LaunchConditionType               `yaml:"type"`
	Value             string                            `yaml:"value,omitempty"`
	ResponseCondition *ResponseConditionLaunchCondition `yaml:"response_condition,omitempty"`
	ParameterNames    []string                          `yaml:"parameter_names,omitempty"`
}

type ResponseConditionLaunchCondition struct {
	Contains   string               `yaml:"contains,omitempty"`
	Part       ResponseContainsPart `yaml:"part,omitempty"`
	StatusCode int                  `yaml:"status_code,omitempty"`
}

// Check if the condition is met against a history item
func (rc *ResponseConditionLaunchCondition) Check(history *db.History) bool {
	statusMatch := false
	containsMatch := false

	if rc.StatusCode != 0 {
		if rc.StatusCode == history.StatusCode {
			statusMatch = true
		}
	} else {
		// If no status is defined, assume it's matched
		statusMatch = true
	}

	if rc.Contains != "" {
		var matchAgainst string

		switch rc.Part {
		case Body:
			body, _ := history.ResponseBody()
			matchAgainst = string(body)

		case Headers:
			headersMap, err := history.ResponseHeaders()
			if err != nil {
				log.Error().Err(err).Msg("Failed to get response headers")
			} else {
				for _, values := range headersMap {
					for _, value := range values {
						matchAgainst += value
					}
				}
			}

		case Raw:
			matchAgainst = string(history.RawResponse)
		}

		if strings.Contains(matchAgainst, rc.Contains) {
			containsMatch = true
		}
	} else {
		// If no contains is defined, assume it's matched
		containsMatch = true
	}

	// If both status and contains conditions are met, return true
	return statusMatch && containsMatch
}

func (rc *ResponseConditionLaunchCondition) CheckWebsocketMessage(message *db.WebSocketMessage) bool {
	// Check if the only conditions available are status code and/or headers
	noContainsOrHeaders := rc.Contains == "" || rc.Part == Headers

	if noContainsOrHeaders {
		log.Debug().Msg("Skipping WebSocket message check as the only conditions available are status code and/or headers.")
		return false
	}

	matchAgainst := ""

	switch rc.Part {
	case Body:
		matchAgainst = message.PayloadData

	case Raw:
		matchAgainst = message.PayloadData

	default:
		matchAgainst = message.PayloadData
	}

	if strings.Contains(matchAgainst, rc.Contains) {
		return true
	}

	return false
}

type LaunchConditions struct {
	Operator   Operator          `yaml:"operator"`
	Conditions []LaunchCondition `yaml:"conditions"`
}
