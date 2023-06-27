package generation

type Payload struct {
	IssueCode        string            `yaml:"issue_code"`
	Value            string            `yaml:"value"`
	Vars             []PayloadVariable `yaml:"vars,omitempty"`
	DetectionMethods []DetectionMethod `yaml:"detection_methods"`
	Categories       []string          `yaml:"categories"`
}
