package generation

import (
	"bytes"
	"fmt"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"text/template"
)

type PayloadGenerator struct {
	IssueCode          string             `yaml:"issue_code"`
	DetectionCondition DetectionCondition `yaml:"detection_condition"`
	DetectionMethods   []DetectionMethod  `yaml:"detection_methods"`
	Vars               []PayloadVariable  `yaml:"vars,omitempty"`
	Templates          []string           `yaml:"templates"`
	Categories         []string           `yaml:"categories"`
}

func (generator *PayloadGenerator) BuildPayloads() ([]Payload, error) {
	var payloads []Payload
	for _, tmpl := range generator.Templates {
		vars, _ := GenerateVars(generator.Vars)
		result, err := ApplyVarsToText(tmpl, vars)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to apply vars to template %s", tmpl)
			return nil, fmt.Errorf("failed to apply vars to template: %v", err)
		}
		vars["payload"] = result
		var processedDetectionMethods []DetectionMethod
		err = lib.DeepCopy(generator.DetectionMethods, &processedDetectionMethods)
		if err != nil {
			return nil, fmt.Errorf("failed to copy detection methods: %v", err)
		}
		err = ApplyVarsToDetectionMethods(processedDetectionMethods, vars)
		if err != nil {
			return nil, fmt.Errorf("failed to apply vars to detection methods: %v", err)
		}
		var processedPayloadVars []PayloadVariable
		for k, v := range vars {
			processedPayloadVars = append(processedPayloadVars, PayloadVariable{
				Name:  k,
				Value: v,
			})
		}

		payloads = append(payloads, Payload{
			IssueCode:        generator.IssueCode,
			Value:            result,
			Vars:             processedPayloadVars,
			DetectionMethods: processedDetectionMethods,
			Categories:       generator.Categories,
		})
	}
	return payloads, nil
}

func GenerateVars(variables []PayloadVariable) (map[string]string, error) {
	vars := make(map[string]string)
	funcs := getTemplateFuncs()

	for _, v := range variables {
		t, err := template.New("").Funcs(funcs).Parse(v.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %v", err)
		}

		var buf bytes.Buffer
		err = t.Execute(&buf, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to execute template: %v", err)
		}

		vars[v.Name] = buf.String()
	}

	return vars, nil
}

func ApplyVarsToText(text string, vars map[string]string) (string, error) {
	funcs := getTemplateFuncs()
	t, err := template.New("").Funcs(funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, vars)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
}

func ApplyVarsToTemplates(data *PayloadGenerator, vars map[string]string) error {
	for i, tmpl := range data.Templates {
		result, _ := ApplyVarsToText(tmpl, vars)
		data.Templates[i] = result
	}
	return nil
}

func ApplyVarsToDetectionMethods(methods []DetectionMethod, vars map[string]string) error {
	for i, method := range methods {
		if method.OOBInteraction != nil {
			OOBAddress, err := ApplyVarsToText(method.OOBInteraction.OOBAddress, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to OOB Address: %v", err)
			}
			methods[i].OOBInteraction.OOBAddress = OOBAddress
		}

		if method.ResponseCondition != nil {
			contains, err := ApplyVarsToText(method.ResponseCondition.Contains, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to contains: %v", err)
			}
			methods[i].ResponseCondition.Contains = contains
		}

		if method.Reflection != nil {
			value, err := ApplyVarsToText(method.Reflection.Value, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to value: %v", err)
			}
			methods[i].Reflection.Value = value
		}

		if method.BrowserEvents != nil {
			event, err := ApplyVarsToText(method.BrowserEvents.Event, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to event: %v", err)
			}
			methods[i].BrowserEvents.Event = event

			value, err := ApplyVarsToText(method.BrowserEvents.Value, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to value: %v", err)
			}
			methods[i].BrowserEvents.Value = value
		}

		if method.TimeBased != nil {
			sleep, err := ApplyVarsToText(method.TimeBased.Sleep, vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to sleep: %v", err)
			}
			methods[i].TimeBased.Sleep = sleep
		}

		if method.ResponseCheck != nil {
			check, err := ApplyVarsToText(string(method.ResponseCheck.Check), vars)
			if err != nil {
				return fmt.Errorf("failed to apply vars to check: %v", err)
			}
			methods[i].ResponseCheck.Check = ResponseConditionCheck(check)
		}
	}
	return nil
}
