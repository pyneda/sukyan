package scan

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/lib"
)

const soapEnvelope = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">` +
	`<soap:Body>` +
	`<ns1:GetUser xmlns:ns1="http://testbed.local/soap/sqli">` +
	`<ns1:userId>12345</ns1:userId>` +
	`<ns1:includeDetails>true</ns1:includeDetails>` +
	`<ns1:token>abc-123</ns1:token>` +
	`</ns1:GetUser>` +
	`</soap:Body>` +
	`</soap:Envelope>`

func elementPointOpts() XMLPointOptions {
	return XMLPointOptions{ElementType: InsertionPointTypeXMLElement}
}

func xmlPointNames(points []InsertionPoint) []string {
	names := make([]string, 0, len(points))
	for _, p := range points {
		names = append(names, p.Name)
	}
	return names
}

func findPoint(t *testing.T, points []InsertionPoint, name string) InsertionPoint {
	t.Helper()
	for _, p := range points {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no insertion point named %q; got %v", name, xmlPointNames(points))
	return InsertionPoint{}
}

func TestExtractXMLPointsFindsLeafElementValues(t *testing.T) {
	points := ExtractXMLPoints(soapEnvelope, elementPointOpts())

	want := map[string]string{"userId": "12345", "includeDetails": "true", "token": "abc-123"}
	if len(points) != len(want) {
		t.Fatalf("expected %d points, got %d: %v", len(want), len(points), xmlPointNames(points))
	}
	for name, value := range want {
		p := findPoint(t, points, name)
		if p.Value != value {
			t.Errorf("point %q: expected value %q, got %q", name, value, p.Value)
		}
		if p.Type != InsertionPointTypeXMLElement {
			t.Errorf("point %q: expected type %q, got %q", name, InsertionPointTypeXMLElement, p.Type)
		}
	}
}

func TestExtractXMLPointsSkipsContainerElements(t *testing.T) {
	points := ExtractXMLPoints(soapEnvelope, elementPointOpts())

	for _, p := range points {
		switch p.Name {
		case "Envelope", "Body", "GetUser":
			t.Errorf("container element %q should not be an insertion point", p.Name)
		}
	}
}

func TestExtractXMLPointsRecordsSpanThatRoundTrips(t *testing.T) {
	points := ExtractXMLPoints(soapEnvelope, elementPointOpts())

	for _, p := range points {
		if !p.Span.Valid {
			t.Fatalf("point %q has no span", p.Name)
		}
		got, err := ApplyXMLPointPayload(soapEnvelope, p, p.Value)
		if err != nil {
			t.Fatalf("point %q: %v", p.Name, err)
		}
		if got != soapEnvelope {
			t.Errorf("point %q: replacing a value with itself changed the document:\n%s", p.Name, got)
		}
	}
}

func TestApplyXMLPointPayloadEscapesMarkup(t *testing.T) {
	points := ExtractXMLPoints(soapEnvelope, elementPointOpts())
	payload := `' OR 1=1-- <b>&x`

	got, err := ApplyXMLPointPayload(soapEnvelope, findPoint(t, points, "userId"), payload)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<b>") {
		t.Errorf("payload markup was not escaped: %s", got)
	}
	decoded := decodeLeaf(t, got, "userId")
	if decoded != payload {
		t.Errorf("payload did not survive a parse round-trip: expected %q, got %q", payload, decoded)
	}
}

func TestApplyXMLPointPayloadKeepsQuotesReadable(t *testing.T) {
	points := ExtractXMLPoints(soapEnvelope, elementPointOpts())

	got, err := ApplyXMLPointPayload(soapEnvelope, findPoint(t, points, "token"), `' OR '1'='1`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, `<ns1:token>' OR '1'='1</ns1:token>`) {
		t.Errorf("single quotes in element text should not be escaped, got: %s", got)
	}
}

func TestApplyXMLPointPayloadOnlyTouchesItsOwnOccurrence(t *testing.T) {
	doc := `<root><item>a</item><item>b</item><item>c</item></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d: %v", len(points), xmlPointNames(points))
	}

	got, err := ApplyXMLPointPayload(doc, points[1], "PAYLOAD")
	if err != nil {
		t.Fatal(err)
	}

	want := `<root><item>a</item><item>PAYLOAD</item><item>c</item></root>`
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestApplyXMLPointPayloadDoesNotMatchPrefixedSiblingNames(t *testing.T) {
	doc := `<root><user>a</user><username>b</username></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	got, err := ApplyXMLPointPayload(doc, findPoint(t, points, "user"), "X")
	if err != nil {
		t.Fatal(err)
	}

	want := `<root><user>X</user><username>b</username></root>`
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestApplyXMLPointPayloadTreatsPayloadLiterally(t *testing.T) {
	doc := `<root><a>1</a></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	got, err := ApplyXMLPointPayload(doc, points[0], `${1}$0\1`)
	if err != nil {
		t.Fatal(err)
	}

	want := `<root><a>${1}$0\1</a></root>`
	if got != want {
		t.Errorf("regex-like payload was expanded: expected %q, got %q", want, got)
	}
}

func TestExtractXMLPointsDisambiguatesRepeatedNames(t *testing.T) {
	doc := `<order><billTo><id>1</id></billTo><shipTo><id>2</id></shipTo><ref>r</ref></order>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	findPoint(t, points, "ref")
	findPoint(t, points, "/order/billTo/id")
	findPoint(t, points, "/order/shipTo/id")
}

func TestExtractXMLPointsIndexesRepeatedSiblings(t *testing.T) {
	doc := `<root><item>a</item><item>b</item></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	findPoint(t, points, "/root/item[1]")
	findPoint(t, points, "/root/item[2]")
}

func TestExtractXMLPointsHandlesEmptyElements(t *testing.T) {
	doc := `<root><empty></empty></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	p := findPoint(t, points, "empty")
	if p.Value != "" {
		t.Errorf("expected empty value, got %q", p.Value)
	}
	got, err := ApplyXMLPointPayload(doc, p, "X")
	if err != nil {
		t.Fatal(err)
	}
	if want := `<root><empty>X</empty></root>`; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractXMLPointsSkipsSelfClosingElements(t *testing.T) {
	doc := `<root><a/><b>keep</b></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	if names := xmlPointNames(points); len(names) != 1 || names[0] != "b" {
		t.Errorf("expected only the b element, got %v", names)
	}
}

func TestExtractXMLPointsIgnoresCommentsAndProcessingInstructions(t *testing.T) {
	doc := `<?xml version="1.0"?><root><!-- <ghost>x</ghost> --><real>y</real></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	if names := xmlPointNames(points); len(names) != 1 || names[0] != "real" {
		t.Errorf("expected only the real element, got %v", names)
	}
}

func TestExtractXMLPointsHandlesCDATA(t *testing.T) {
	doc := `<root><bio><![CDATA[hello <world>]]></bio></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	p := findPoint(t, points, "bio")
	if p.Value != "hello <world>" {
		t.Errorf("expected decoded CDATA text, got %q", p.Value)
	}
	got, err := ApplyXMLPointPayload(doc, p, "payload")
	if err != nil {
		t.Fatal(err)
	}
	if decodeLeaf(t, got, "bio") != "payload" {
		t.Errorf("CDATA replacement did not round-trip: %s", got)
	}
}

func TestExtractXMLPointsDecodesEntityReferences(t *testing.T) {
	doc := `<root><a>a&amp;b</a></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	p := points[0]
	if p.Value != "a&b" {
		t.Errorf("expected decoded value %q, got %q", "a&b", p.Value)
	}
	got, err := ApplyXMLPointPayload(doc, p, p.Value)
	if err != nil {
		t.Fatal(err)
	}
	if got != doc {
		t.Errorf("expected re-encoded document %q, got %q", doc, got)
	}
}

func TestExtractXMLPointsGuessesValueType(t *testing.T) {
	doc := `<root><target>https://example.com/x</target></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	if got := points[0].ValueType; got != lib.TypeURL {
		t.Errorf("expected %q, got %q", lib.TypeURL, got)
	}
}

func TestExtractXMLPointsRespectsMaxPoints(t *testing.T) {
	var b strings.Builder
	b.WriteString("<root>")
	for i := 0; i < 50; i++ {
		b.WriteString("<f>v</f>")
	}
	b.WriteString("</root>")

	opts := elementPointOpts()
	opts.MaxPoints = 25
	points := ExtractXMLPoints(b.String(), opts)

	if len(points) != 25 {
		t.Errorf("expected the cap to apply, got %d points", len(points))
	}
}

func TestExtractXMLPointsReturnsNothingForMalformedXML(t *testing.T) {
	for _, doc := range []string{`<root><a>1</b></root>`, `not xml at all`, ``, `<root>`} {
		if points := ExtractXMLPoints(doc, elementPointOpts()); len(points) != 0 {
			t.Errorf("expected no points for %q, got %v", doc, xmlPointNames(points))
		}
	}
}

func TestExtractXMLPointsOmitsAttributesByDefault(t *testing.T) {
	doc := `<root><a id="7">v</a></root>`
	points := ExtractXMLPoints(doc, elementPointOpts())

	if names := xmlPointNames(points); len(names) != 1 || names[0] != "a" {
		t.Errorf("expected only the element value, got %v", names)
	}
}

func TestExtractXMLPointsExtractsAttributeValuesWhenEnabled(t *testing.T) {
	doc := `<root><a id="7" name='bob'>v</a></root>`
	opts := elementPointOpts()
	opts.IncludeAttributes = true
	opts.AttributeType = InsertionPointTypeXMLAttribute
	points := ExtractXMLPoints(doc, opts)

	id := findPoint(t, points, "id")
	if id.Value != "7" || id.Type != InsertionPointTypeXMLAttribute {
		t.Errorf("unexpected attribute point: %+v", id)
	}
	got, err := ApplyXMLPointPayload(doc, id, "9")
	if err != nil {
		t.Fatal(err)
	}
	if want := `<root><a id="9" name='bob'>v</a></root>`; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestExtractXMLPointsSkipsNamespaceDeclarations(t *testing.T) {
	opts := elementPointOpts()
	opts.IncludeAttributes = true
	opts.AttributeType = InsertionPointTypeXMLAttribute
	points := ExtractXMLPoints(soapEnvelope, opts)

	for _, p := range points {
		if strings.Contains(p.Name, "xmlns") {
			t.Errorf("namespace declaration should not be an insertion point: %q", p.Name)
		}
	}
}

func TestApplyXMLPointPayloadEscapesTheQuoteCharInAttributes(t *testing.T) {
	doc := `<root><a id="7">v</a></root>`
	opts := elementPointOpts()
	opts.IncludeAttributes = true
	opts.AttributeType = InsertionPointTypeXMLAttribute
	points := ExtractXMLPoints(doc, opts)

	got, err := ApplyXMLPointPayload(doc, findPoint(t, points, "id"), `x" onload="alert(1)`)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, `onload="alert(1)"`) {
		t.Errorf("attribute payload broke out of its quotes: %s", got)
	}
	if err := xml.Unmarshal([]byte(got), new(struct{})); err != nil {
		t.Errorf("attribute replacement produced malformed XML: %v (%s)", err, got)
	}
}

func TestApplyXMLPointPayloadRejectsPointsWithoutSpan(t *testing.T) {
	doc := `<root><a>1</a></root>`
	point := InsertionPoint{Type: InsertionPointTypeXMLElement, Name: "a", Value: "1"}

	if _, err := ApplyXMLPointPayload(doc, point, "x"); err == nil {
		t.Fatal("expected an error for a point carrying no span")
	}
}

func TestApplyXMLPointPayloadRejectsSpanOutsideDocument(t *testing.T) {
	doc := `<root><a>1</a></root>`
	point := InsertionPoint{
		Type: InsertionPointTypeXMLElement,
		Name: "a",
		Span: InsertionPointSpan{Start: 500, End: 600, Valid: true},
	}

	if _, err := ApplyXMLPointPayload(doc, point, "x"); err == nil {
		t.Fatal("expected an error for a span outside the document")
	}
}

// decodeLeaf returns the text content of the first element with the given local name.
func decodeLeaf(t *testing.T, doc, localName string) string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(doc))
	for {
		token, err := decoder.Token()
		if err != nil {
			t.Fatalf("document is not well-formed: %v (%s)", err, doc)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != localName {
			continue
		}
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			t.Fatalf("decoding %s: %v", localName, err)
		}
		return text
	}
}

func TestExtractXMLPointsReturnsNothingForDoctypeWithUndefinedEntities(t *testing.T) {
	doc := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><r><a>&xxe;</a></r>`

	points := ExtractXMLPoints(doc, elementPointOpts())

	if len(points) != 0 {
		t.Errorf("an entity-bearing document must fall back to the whole-body surface, got %v", xmlPointNames(points))
	}
}

func TestExtractXMLPointsBoundsWorkOnHugeDocuments(t *testing.T) {
	var b strings.Builder
	b.WriteString("<root>")
	for i := 0; i < 200000; i++ {
		b.WriteString("<f>v</f>")
	}
	b.WriteString("</root>")

	opts := elementPointOpts()
	opts.MaxPoints = 25

	start := time.Now()
	points := ExtractXMLPoints(b.String(), opts)
	elapsed := time.Since(start)

	if len(points) != 25 {
		t.Errorf("expected the cap to hold on a huge document, got %d", len(points))
	}
	// Walking every leaf of a hostile document before discarding all but 25 of them is
	// work an attacker controls, so extraction stops once the cap can no longer change.
	if elapsed > 2*time.Second {
		t.Errorf("extraction walked the whole document: took %s", elapsed)
	}
}

func TestExtractXMLPointsHandlesDeeplyNestedDocuments(t *testing.T) {
	depth := 5000
	doc := strings.Repeat("<a>", depth) + "leaf" + strings.Repeat("</a>", depth)

	points := ExtractXMLPoints(doc, elementPointOpts())

	if len(points) != 1 || points[0].Value != "leaf" {
		t.Errorf("expected one deep leaf point, got %d: %v", len(points), xmlPointNames(points))
	}
}
