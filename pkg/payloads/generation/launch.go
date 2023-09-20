package generation

type LaunchConditionType string

const (
	Platform               LaunchConditionType = "platform"
	ScanMode               LaunchConditionType = "scan_mode"
	ParameterValueDataType LaunchConditionType = "parameter_value_data_type"
	ResponseCondition      LaunchConditionType = "response_condition"
)

type LaunchCondition struct {
	Type              LaunchConditionType               `yaml:"type"`
	Value             string                            `yaml:"value,omitempty"`
	ResponseCondition *ResponseConditionLaunchCondition `yaml:"response_condition,omitempty"`
}

type ResponseConditionLaunchCondition struct {
	Contains   string               `yaml:"contains,omitempty"`
	Part       ResponseContainsPart `yaml:"part,omitempty"`
	StatusCode int                  `yaml:"status_code,omitempty"`
}

type LaunchConditions struct {
	Operator   Operator          `yaml:"operator"`
	Conditions []LaunchCondition `yaml:"conditions"`
}
