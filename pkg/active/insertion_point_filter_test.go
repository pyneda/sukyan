package active

import (
	"testing"

	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

func filterTestPoints() []scan.InsertionPoint {
	return []scan.InsertionPoint{
		{Type: scan.InsertionPointTypeParameter, Name: "q"},
		{Type: scan.InsertionPointTypeBody, Name: "field"},
		{Type: scan.InsertionPointTypeFullBody, Name: "xml", ValueType: lib.TypeXML},
		{Type: scan.InsertionPointTypeXMLElement, Name: "userId"},
		{Type: scan.InsertionPointTypeGraphQLVariable, Name: "gqlVar"},
		{Type: scan.InsertionPointTypeGraphQLInlineArg, Name: "gqlArg"},
		{Type: scan.InsertionPointTypeHeader, Name: "X-Static"},
		{Type: scan.InsertionPointTypeCookie, Name: "session", Behaviour: scan.InsertionPointBehaviour{IsDynamic: true}},
		{Type: scan.InsertionPointTypeURLPath, Name: "id", Behaviour: scan.InsertionPointBehaviour{IsReflected: true}},
	}
}

func selectedNames(points []scan.InsertionPoint) map[string]bool {
	names := make(map[string]bool, len(points))
	for _, p := range points {
		names[p.Name] = true
	}
	return names
}

func TestInsertionPointsForModeSmartKeepsXMLElementPoints(t *testing.T) {
	got := selectedNames(insertionPointsForMode(scan_options.ScanModeSmart, filterTestPoints()))

	for _, name := range []string{"q", "field", "xml", "userId", "gqlVar", "gqlArg", "session", "id"} {
		if !got[name] {
			t.Errorf("smart mode should audit %q", name)
		}
	}
	if got["X-Static"] {
		t.Error("smart mode should skip a static header point")
	}
}

func TestInsertionPointsForModeFastKeepsOnlyDynamicOrReflected(t *testing.T) {
	got := selectedNames(insertionPointsForMode(scan_options.ScanModeFast, filterTestPoints()))

	if len(got) != 2 || !got["session"] || !got["id"] {
		t.Errorf("fast mode should keep only dynamic/reflected points, got %v", got)
	}
}

func TestInsertionPointsForModeFuzzKeepsEverything(t *testing.T) {
	points := filterTestPoints()

	if got := insertionPointsForMode(scan_options.ScanModeFuzz, points); len(got) != len(points) {
		t.Errorf("fuzz mode should keep all %d points, got %d", len(points), len(got))
	}
}
