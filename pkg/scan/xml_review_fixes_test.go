package scan

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
)

// Attribute points must keep the bare attribute name: payload templates gate on
// insertion_point_name, and an "@" prefix silently excluded every one of them.
func TestExtractXMLPointsNamesAttributesWithoutAPrefix(t *testing.T) {
	doc := `<root><a username="bob">v</a></root>`
	opts := elementPointOpts()
	opts.IncludeAttributes = true
	opts.AttributeType = InsertionPointTypeXMLAttribute

	points := ExtractXMLPoints(doc, opts)

	p := findPoint(t, points, "username")
	if p.Type != InsertionPointTypeXMLAttribute || p.Value != "bob" {
		t.Errorf("unexpected attribute point: %+v", p)
	}
}

func TestExtractXMLPointsDisambiguatesAnAttributeSharingAnElementName(t *testing.T) {
	doc := `<root><id>1</id><a id="2">v</a></root>`
	opts := elementPointOpts()
	opts.IncludeAttributes = true
	opts.AttributeType = InsertionPointTypeXMLAttribute

	points := ExtractXMLPoints(doc, opts)

	findPoint(t, points, "/root/a/@id")
	findPoint(t, points, "/root/id")
}

// The WebSocket extractor prepends a synthetic whole-document point named "document".
// A leaf element of the same name would collide with it in the issue-dedup key and
// steal xxe.yaml's insertion_point_name gate.
func TestExtractXMLPointsAvoidsReservedNames(t *testing.T) {
	doc := `<root><document>text</document></root>`
	opts := elementPointOpts()
	opts.ReservedNames = []string{"document"}

	points := ExtractXMLPoints(doc, opts)

	findPoint(t, points, "/root/document")
}

func TestWebSocketXMLLeafNamedDocumentDoesNotCollideWithTheWholeDocumentPoint(t *testing.T) {
	points := wsXMLPoints(t, `<root><document>text</document></root>`)

	seen := map[string]int{}
	for _, p := range points {
		seen[p.Name]++
	}
	if seen["document"] != 1 {
		t.Errorf("expected exactly one point named \"document\" (the whole-message one), got %d: %v", seen["document"], xmlPointNames(points))
	}
}

// Characters outside the XML Char production make the whole document unparseable, so
// the probe tests nothing. ldap-injection.yaml ships payloads containing literal NULs.
func TestApplyXMLPointPayloadDropsCharactersXMLCannotRepresent(t *testing.T) {
	doc := `<root><username>bob</username></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	got, err := ApplyXMLPointPayload(doc, points[0], "*)(cn=*\x00")
	if err != nil {
		t.Fatal(err)
	}

	if strings.ContainsRune(got, 0) {
		t.Errorf("NUL survived into the document: %q", got)
	}
	if err := xml.Unmarshal([]byte(got), new(struct{})); err != nil {
		t.Errorf("document is not well-formed after splicing: %v (%q)", err, got)
	}
	if !strings.Contains(got, "*)(cn=*") {
		t.Errorf("the representable part of the payload must survive: %q", got)
	}
}

func TestApplyXMLPointPayloadKeepsTabsAndNewlines(t *testing.T) {
	doc := `<root><a>v</a></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	got, err := ApplyXMLPointPayload(doc, points[0], "one\ttwo\nthree")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "one\ttwo\nthree") {
		t.Errorf("legal whitespace must be preserved: %q", got)
	}
}

// Two builders addressing the same span used to splice the second payload into the
// bytes the first one wrote, producing a corrupt document and a wasted probe.
func TestCreateRequestFromInsertionPointsRejectsOverlappingXMLSpans(t *testing.T) {
	history := xmlHistory("text/xml", `<r><targetPort>8080</targetPort></r>`)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}
	target := elementPoints(points)[0]

	body := requestBodyString(t, history, []InsertionPointBuilder{
		{Point: target, Payload: "http://127.0.0.1:1/"},
		{Point: target, Payload: "1"},
	})

	if want := `<r><targetPort>http://127.0.0.1:1/</targetPort></r>`; body != want {
		t.Errorf("expected the first payload to win intact, got %q", body)
	}
}

// A urlencoded body that happens to contain XML gets a generic "fullbody" point whose
// guessed value type is XML. Gating it would silently disable the whole template
// library for that request.
func TestShouldLaunchDoesNotGateAMislabelledFullBodyPoint(t *testing.T) {
	scanner := &TemplateScanner{}
	generator := &generation.PayloadGenerator{ID: "sqli-error"}
	point := InsertionPoint{
		Type:      InsertionPointTypeFullBody,
		Name:      "fullbody",
		Value:     `<order><cmd>ls</cmd></order>`,
		ValueType: lib.TypeXML,
	}

	if !scanner.shouldLaunch(&db.History{}, generator, point, launchOptions()) {
		t.Error("only the XML body point named \"xml\" carries the whole-document surface")
	}
}
