package scan

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
)

func launchOptions() options.HistoryItemScanOptions {
	return options.HistoryItemScanOptions{Mode: options.ScanModeSmart}
}

func xmlWholeBodyPoint() InsertionPoint {
	return InsertionPoint{
		Type:         InsertionPointTypeFullBody,
		Name:         "xml",
		Value:        soapEnvelope,
		ValueType:    lib.TypeXML,
		OriginalData: soapEnvelope,
	}
}

func xmlElementPoint() InsertionPoint {
	return InsertionPoint{
		Type:      InsertionPointTypeXMLElement,
		Name:      "userId",
		Value:     "12345",
		ValueType: lib.TypeInt,
		Span:      InsertionPointSpan{Start: 1, End: 2, Valid: true},
	}
}

// Value-injection templates carry no launch block, so before the whole-body gate they
// all fired at the XML body and replaced the envelope with a bare payload.
func TestShouldLaunchSkipsUndeclaredGeneratorsOnTheXMLWholeBodyPoint(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{ID: "sqli-error"}

	if scanner.shouldLaunch(&db.History{}, generator, xmlWholeBodyPoint(), launchOptions()) {
		t.Error("a generator that does not declare the whole-body XML surface must not launch on it")
	}
}

func TestShouldLaunchAllowsGeneratorsThatNameTheXMLWholeBodyPoint(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{
		ID: "xxe",
		Launch: generation.LaunchConditions{
			Operator: generation.Or,
			Conditions: []generation.LaunchCondition{
				{Type: generation.ParameterName, ParameterNames: []string{"xml", "data"}},
			},
		},
	}

	if !scanner.shouldLaunch(&db.History{}, generator, xmlWholeBodyPoint(), launchOptions()) {
		t.Error("xxe declares insertion_point_name: xml and must still launch on the whole-body point")
	}
}

func TestShouldLaunchAllowsGeneratorsThatDeclareTheInsertionPointType(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{
		ID: "xml-bomb",
		Launch: generation.LaunchConditions{
			Operator: generation.Or,
			Conditions: []generation.LaunchCondition{
				{Type: generation.InsertionPointTypeCondition, Value: string(InsertionPointTypeFullBody)},
			},
		},
	}

	if !scanner.shouldLaunch(&db.History{}, generator, xmlWholeBodyPoint(), launchOptions()) {
		t.Error("a generator declaring insertion_point_type: fullbody must launch on the whole-body point")
	}
}

func TestShouldLaunchStillRunsUndeclaredGeneratorsOnXMLElementPoints(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{ID: "sqli-error"}

	if !scanner.shouldLaunch(&db.History{}, generator, xmlElementPoint(), launchOptions()) {
		t.Error("value-injection generators must reach per-element XML points")
	}
}

func TestShouldLaunchLeavesNonXMLWholeBodyPointsAlone(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{ID: "sqli-error"}
	point := InsertionPoint{
		Type:      InsertionPointTypeFullBody,
		Name:      "fullbody",
		Value:     `{"a":"1"}`,
		ValueType: lib.TypeJSON,
	}

	if !scanner.shouldLaunch(&db.History{}, generator, point, launchOptions()) {
		t.Error("the gate must only apply to XML whole-body points")
	}
}

func TestShouldLaunchMatchesTheInsertionPointTypeCondition(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{
		ID: "element-only",
		Launch: generation.LaunchConditions{
			Operator: generation.And,
			Conditions: []generation.LaunchCondition{
				{Type: generation.InsertionPointTypeCondition, Value: string(InsertionPointTypeXMLElement)},
			},
		},
	}

	if !scanner.shouldLaunch(&db.History{}, generator, xmlElementPoint(), launchOptions()) {
		t.Error("expected the condition to match an xml_element point")
	}
	if scanner.shouldLaunch(&db.History{}, generator, InsertionPoint{Type: InsertionPointTypeParameter, Name: "q"}, launchOptions()) {
		t.Error("expected the condition not to match a query parameter point")
	}
}
