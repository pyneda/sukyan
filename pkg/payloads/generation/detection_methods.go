package generation

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
	Contains   string               `yaml:"contains,omitempty"`
	Part       ResponseContainsPart `yaml:"part,omitempty"`
	StatusCode int                  `yaml:"status_code,omitempty"`
	Confidence int                  `yaml:"confidence,omitempty"`
}

type ResponseCheckDetectionMethod struct {
	Check      ResponseConditionCheck `yaml:"check"`
	Confidence int                    `yaml:"confidence,omitempty"`
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
