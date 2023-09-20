package generation

import (
	"fmt"
	"github.com/pyneda/sukyan/lib/integrations"
)

type Payload struct {
	IssueCode          string            `yaml:"issue_code"`
	Value              string            `yaml:"value"`
	Vars               []PayloadVariable `yaml:"vars,omitempty"`
	DetectionCondition Operator          `yaml:"detection_condition"`
	DetectionMethods   []DetectionMethod `yaml:"detection_methods"`
	Categories         []string          `yaml:"categories"`
	InteractionDomain  integrations.InteractionDomain
}

func (payload *Payload) Print() {
	fmt.Printf("Payload:\n")
	fmt.Println(payload.Value)
	fmt.Println("\nDetection Methods:")
	for _, dm := range payload.DetectionMethods {
		fmt.Println(dm.GetMethod())
	}
	fmt.Println("\nVars:")
	for _, v := range payload.Vars {
		fmt.Printf("%s: %s\n", v.Name, v.Value)
	}
	if payload.InteractionDomain.URL != "" {
		fmt.Printf("\nInteraction URL: %s\n", payload.InteractionDomain.URL)
	}

}
