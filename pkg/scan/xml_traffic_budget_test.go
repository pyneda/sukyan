package scan

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
)

// payloadCountForPoint reports how many payloads the real embedded template library
// would fire at a single insertion point, which is the unit of scan traffic.
func payloadCountForPoint(t *testing.T, history *db.History, point InsertionPoint) int {
	t.Helper()
	generators, err := generation.LoadGenerators("")
	if err != nil {
		t.Fatalf("loading generators: %v", err)
	}

	scanner := &TemplateScanner{}
	opts := options.HistoryItemScanOptions{Mode: options.ScanModeSmart}
	total := 0
	for _, generator := range generators {
		if scanner.shouldLaunch(history, generator, point, opts) {
			total += len(generator.Templates)
		}
	}
	return total
}

// Every value-injection template used to fire at the XML whole-body point, replacing
// the envelope with a bare payload and collecting ~200 parse faults per SOAP request.
func TestXMLWholeBodyPointCostsFewPayloads(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	got := payloadCountForPoint(t, history, findXMLBodyPoints(points)[0])

	if got == 0 {
		t.Fatal("the whole-body point must still receive whole-document payloads (XXE)")
	}
	if got > 20 {
		t.Errorf("the whole-body XML point should only receive whole-document payloads, got %d", got)
	}
}

func TestXMLElementPointsReceiveTheValueInjectionLibrary(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	got := payloadCountForPoint(t, history, elementPoints(points)[0])

	if got < 100 {
		t.Errorf("expected the value-injection library to reach XML element points, got %d payloads", got)
	}
}

// The per-element surface is what costs traffic, so the extraction cap is the control.
func TestXMLBodyTrafficStaysBounded(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	total := 0
	for _, point := range points {
		if point.Type == InsertionPointTypeXMLElement || isXMLWholeBodyPoint(point) {
			total += payloadCountForPoint(t, history, point)
		}
	}

	// Three fuzzable parameters plus the XXE surface. Before per-element points this
	// same request cost one whole-body sweep of the entire library and reached nothing.
	perElement := payloadCountForPoint(t, history, elementPoints(points)[0])
	if max := 4 * perElement; total > max {
		t.Errorf("XML body traffic %d exceeds the expected ceiling %d", total, max)
	}
	t.Logf("SOAP envelope with 3 parameters: %d payloads across %d XML points", total, len(elementPoints(points))+1)
}
