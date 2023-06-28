package generation

import (
	"github.com/pyneda/sukyan/lib/integrations"
	"strings"
	"testing"
	"time"
)

func TestGenerateVars(t *testing.T) {
	testCases := []struct {
		name        string
		input       []PayloadVariable
		expectError bool
		expectVals  map[string]string
	}{
		{
			name: "Valid vars",
			input: []PayloadVariable{
				{
					Name:  "var1",
					Value: "{{genInteractionAddress}}",
				},
				{
					Name:  "var2",
					Value: "{{genRandInt 1 9}}",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid var - incorrect function name",
			input: []PayloadVariable{
				{
					Name:  "var1",
					Value: "{{nonExistentFunction}}",
				},
			},
			expectError: true,
		},
		{
			name: "Fixed randInt",
			input: []PayloadVariable{
				{
					Name:  "var1",
					Value: "{{genRandInt 1 1}}",
				},
			},
			expectError: false,
			expectVals: map[string]string{
				"var1": "1",
			},
		},
	}
	manager := integrations.InteractionsManager{
		GetAsnInfo:      false,
		PollingInterval: time.Duration(60 * time.Second),
	}
	manager.Start()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := GenerateVars(tc.input, manager)
			if (err != nil) != tc.expectError {
				t.Errorf("GenerateVars() error = %v, expectError %v", err, tc.expectError)
				return
			}

			if err == nil {
				for k, v := range got {
					if strings.Contains(v, "{{") || strings.Contains(v, "}}") {
						t.Errorf("Raw template detected in key %s, value %s", k, v)
					}

					if expectVal, ok := tc.expectVals[k]; ok {
						if v != expectVal {
							t.Errorf("Expected value for key %s was %s, got %s instead", k, expectVal, v)
						}
					}
				}
			}
		})
	}
}

func TestApplyVarsToText(t *testing.T) {
	testCases := []struct {
		name        string
		text        string
		vars        map[string]string
		expectError bool
		expectText  string
	}{
		{
			name: "Valid vars",
			text: "Hello, {{.Name}}. Your ID is {{.ID}}.",
			vars: map[string]string{
				"Name": "John",
				"ID":   "1",
			},
			expectError: false,
			expectText:  "Hello, John. Your ID is 1.",
		},
		{
			name: "Invalid var - incorrect function name",
			text: "Hello, {{.Name}}. Your ID is {{nonExistentFunction}}.",
			vars: map[string]string{
				"Name": "John",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyVarsToText(tc.text, tc.vars)
			if (err != nil) != tc.expectError {
				t.Errorf("ApplyVarsToText() error = %v, expectError %v", err, tc.expectError)
				return
			}

			if err == nil && got != tc.expectText {
				t.Errorf("Expected text was %s, got %s instead", tc.expectText, got)
			}
		})
	}
}

func TestApplyVarsToDetectionMethods(t *testing.T) {
	testCases := []struct {
		name        string
		methods     []DetectionMethod
		vars        map[string]string
		expectError bool
	}{
		{
			name: "Test case 1: All field of methods use variable",
			methods: []DetectionMethod{
				{
					OOBInteraction: &OOBInteractionDetectionMethod{
						OOBAddress: "{{.testVar1}}",
					},
					ResponseCondition: &ResponseConditionDetectionMethod{
						Contains: "{{.testVar2}}",
					},
					Reflection: &ReflectionDetectionMethod{
						Value: "{{.testVar3}}",
					},
					BrowserEvents: &BrowserEventsDetectionMethod{
						Event: "{{.testVar4}}",
						Value: "{{.testVar5}}",
					},
					TimeBased: &TimeBasedDetectionMethod{
						Sleep: "{{.testVar6}}",
					},
					ResponseCheck: &ResponseCheckDetectionMethod{
						Check: ResponseConditionCheck("{{.testVar7}}"),
					},
				},
			},
			vars: map[string]string{
				"testVar1": "value1",
				"testVar2": "value2",
				"testVar3": "value3",
				"testVar4": "value4",
				"testVar5": "value5",
				"testVar6": "value6",
				"testVar7": "value7",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ApplyVarsToDetectionMethods(tc.methods, tc.vars)
			if (err != nil) != tc.expectError {
				t.Errorf("ApplyVarsToDetectionMethods() error = %v, expectError %v", err, tc.expectError)
				return
			}
			for _, method := range tc.methods {
				if method.OOBInteraction != nil && method.OOBInteraction.OOBAddress != tc.vars["testVar1"] {
					t.Errorf("OOBAddress does not match expected value")
				}
				if method.ResponseCondition != nil && method.ResponseCondition.Contains != tc.vars["testVar2"] {
					t.Errorf("Contains does not match expected value")
				}
				if method.Reflection != nil && method.Reflection.Value != tc.vars["testVar3"] {
					t.Errorf("Value does not match expected value")
				}
				if method.BrowserEvents != nil {
					if method.BrowserEvents.Event != tc.vars["testVar4"] {
						t.Errorf("Event does not match expected value")
					}
					if method.BrowserEvents.Value != tc.vars["testVar5"] {
						t.Errorf("Value does not match expected value")
					}
				}
				if method.TimeBased != nil && method.TimeBased.Sleep != tc.vars["testVar6"] {
					t.Errorf("Sleep does not match expected value")
				}
				if method.ResponseCheck != nil && string(method.ResponseCheck.Check) != tc.vars["testVar7"] {
					t.Errorf("Check does not match expected value")
				}
			}
		})
	}
}
