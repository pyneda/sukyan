package generation

import (
	"strconv"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type DetectionMethod struct {
	OOBInteraction    *OOBInteractionDetectionMethod    `yaml:"oob_interaction,omitempty"`
	ResponseCondition *ResponseConditionDetectionMethod `yaml:"response_condition,omitempty"`
	Reflection        *ReflectionDetectionMethod        `yaml:"reflection,omitempty"`
	BrowserEvents     *BrowserEventsDetectionMethod     `yaml:"browser_events,omitempty"`
	TimeBased         *TimeBasedDetectionMethod         `yaml:"time_based,omitempty"`
	ResponseCheck     *ResponseCheckDetectionMethod     `yaml:"response_check,omitempty"`
}

func (dm *DetectionMethod) GetMethod() interface{} {
	if dm.OOBInteraction != nil {
		return dm.OOBInteraction
	}
	if dm.ResponseCondition != nil {
		return dm.ResponseCondition
	}
	if dm.Reflection != nil {
		return dm.Reflection
	}
	if dm.BrowserEvents != nil {
		return dm.BrowserEvents
	}
	if dm.TimeBased != nil {
		return dm.TimeBased
	}
	if dm.ResponseCheck != nil {
		return dm.ResponseCheck
	}
	return nil
}

type OOBInteractionDetectionMethod struct {
	OOBAddress string `yaml:"oob_address"`
	Confidence int    `yaml:"confidence,omitempty"`
}

type ResponseContainsPart string

const (
	Body    ResponseContainsPart = "body"
	Headers ResponseContainsPart = "headers"
	Raw     ResponseContainsPart = "raw"
)

type ResponseConditionDetectionMethod struct {
	Contains               string               `yaml:"contains,omitempty"`
	Part                   ResponseContainsPart `yaml:"part,omitempty"`
	StatusCode             int                  `yaml:"status_code,omitempty"`
	StatusCodeShouldChange bool                 `yaml:"status_chode_should_change,omitempty"`
	Confidence             int                  `yaml:"confidence,omitempty"`
	// TODO: Add support for the issue override
	IssueOverride db.IssueCode `yaml:"issue_override,omitempty"`
}

type ResponseCheckDetectionMethod struct {
	Check         ResponseConditionCheck `yaml:"check"`
	Confidence    int                    `yaml:"confidence,omitempty"`
	IssueOverride db.IssueCode           `yaml:"issue_override,omitempty"`
}

type ReflectionDetectionMethod struct {
	Value      string `yaml:"value,omitempty"`
	Confidence int    `yaml:"confidence,omitempty"`
}

type BrowserEventsDetectionMethod struct {
	Event      string `yaml:"event"`
	Value      string `yaml:"value"`
	Confidence int    `yaml:"confidence,omitempty"`
}

type TimeBasedDetectionMethod struct {
	Sleep      string `yaml:"sleep"`
	Confidence int    `yaml:"confidence,omitempty"`
}

func (t *TimeBasedDetectionMethod) ParseSleepDuration(sleep string) time.Duration {
	// try to parse the string directly as a duration (this handles formats like "5s", "100ms", "1.5m", etc)
	duration, err := time.ParseDuration(sleep)
	if err == nil {
		return duration
	}

	sleepInt, err := strconv.Atoi(sleep)
	if err != nil {
		log.Error().Err(err).Str("sleep", sleep).Msg("Error converting sleep string to int")
		return 0
	}
	var sleepDuration time.Duration
	if sleepInt >= 1000 {
		sleepDuration = time.Duration(sleepInt) * time.Millisecond
	} else {
		sleepDuration = time.Duration(sleepInt) * time.Second
	}
	return sleepDuration
}

func (t *TimeBasedDetectionMethod) CheckIfResultDurationIsHigher(resultDuration time.Duration) bool {
	sleepDuration := t.ParseSleepDuration(t.Sleep)

	if sleepDuration == 0 {
		log.Warn().Str("sleep", t.Sleep).Msg("Invalid sleep duration, cannot compare")
		return false
	}

	log.Debug().
		Dur("actual", resultDuration).
		Dur("expected", sleepDuration).
		Msg("Comparing time-based detection durations")

	return sleepDuration != 0 && resultDuration >= sleepDuration
}
