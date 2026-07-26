package scan

import (
	"io"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func xmlHistory(contentType, body string) *db.History {
	return &db.History{
		URL:                "http://example.com/soap/sqli",
		Method:             "POST",
		RawRequest:         []byte("POST /soap/sqli HTTP/1.1\r\nHost: example.com\r\nX-Trace: t1\r\nContent-Type: " + contentType + "\r\n\r\n" + body),
		RequestContentType: contentType,
	}
}

func elementPoints(points []InsertionPoint) []InsertionPoint {
	var matches []InsertionPoint
	for _, p := range points {
		if p.Type == InsertionPointTypeXMLElement {
			matches = append(matches, p)
		}
	}
	return matches
}

func TestGetInsertionPointsEmitsXMLElementPointsForSOAPBody(t *testing.T) {
	points, err := GetInsertionPoints(xmlHistory("text/xml", soapEnvelope), []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	got := elementPoints(points)
	if len(got) != 3 {
		t.Fatalf("expected 3 XML element points, got %d: %v", len(got), xmlPointNames(got))
	}
	for _, p := range got {
		if !p.Span.Valid {
			t.Errorf("point %q has no span", p.Name)
		}
		if p.OriginalData != soapEnvelope {
			t.Errorf("point %q: OriginalData must be the whole body", p.Name)
		}
	}
}

func TestGetInsertionPointsKeepsTheXMLWholeBodyPointAlongsideElementPoints(t *testing.T) {
	points, err := GetInsertionPoints(xmlHistory("text/xml", soapEnvelope), []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(findXMLBodyPoints(points)); n != 1 {
		t.Errorf("expected the XXE whole-body point to survive, got %d", n)
	}
}

func TestGetInsertionPointsEmitsXMLElementPointsForSOAP12ContentType(t *testing.T) {
	points, err := GetInsertionPoints(xmlHistory("application/soap+xml; charset=utf-8", soapEnvelope), []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(elementPoints(points)); n != 3 {
		t.Errorf("expected 3 element points for a SOAP 1.2 body, got %d", n)
	}
	if n := len(findXMLBodyPoints(points)); n != 1 {
		t.Errorf("expected one whole-body point for a SOAP 1.2 body, got %d", n)
	}
}

func TestGetInsertionPointsSkipsXMLElementPointsWhenOutOfScope(t *testing.T) {
	points, err := GetInsertionPoints(xmlHistory("text/xml", soapEnvelope), []string{"parameters", "urlpath"})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(elementPoints(points)); n != 0 {
		t.Errorf("expected no element points when xml is out of scope, got %d", n)
	}
}

func TestGetInsertionPointsCapsXMLElementPoints(t *testing.T) {
	var b strings.Builder
	b.WriteString("<root>")
	for i := 0; i < defaultMaxXMLInsertionPoints+20; i++ {
		b.WriteString("<f>v</f>")
	}
	b.WriteString("</root>")

	points, err := GetInsertionPoints(xmlHistory("text/xml", b.String()), []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(elementPoints(points)); n != defaultMaxXMLInsertionPoints {
		t.Errorf("expected the cap of %d to apply, got %d element points", defaultMaxXMLInsertionPoints, n)
	}
}

func TestGetInsertionPointsLeavesNonXMLBodiesUntouched(t *testing.T) {
	points, err := GetInsertionPoints(xmlHistory("application/json", `{"a":"1"}`), []string{"xml", "body"})
	if err != nil {
		t.Fatal(err)
	}

	if n := len(elementPoints(points)); n != 0 {
		t.Errorf("a JSON body must not produce XML element points, got %d", n)
	}
}

func requestBodyString(t *testing.T, history *db.History, builders []InsertionPointBuilder) string {
	t.Helper()
	req, err := CreateRequestFromInsertionPoints(history, builders)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if req.Body == nil {
		return ""
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(body)
}

func TestCreateRequestFromInsertionPointsInjectsIntoTheTargetElement(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}
	target := findPoint(t, elementPoints(points), "userId")

	body := requestBodyString(t, history, []InsertionPointBuilder{{Point: target, Payload: "' OR 1=1--"}})

	if !strings.Contains(body, `<ns1:userId>' OR 1=1--</ns1:userId>`) {
		t.Errorf("payload did not land in the target element: %s", body)
	}
	if !strings.Contains(body, `<ns1:token>abc-123</ns1:token>`) {
		t.Errorf("sibling elements must be left intact: %s", body)
	}
	if !strings.Contains(body, `<soap:Envelope`) {
		t.Errorf("the envelope must survive: %s", body)
	}
}

func TestCreateRequestFromInsertionPointsKeepsXMLBodyWhenFuzzingAHeader(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"headers"})
	if err != nil {
		t.Fatal(err)
	}
	target := findPoint(t, points, "X-Trace")

	body := requestBodyString(t, history, []InsertionPointBuilder{{Point: target, Payload: "probe"}})

	if body != soapEnvelope {
		t.Errorf("scanning a header must not drop the XML body\nwant: %s\ngot:  %s", soapEnvelope, body)
	}
}

func TestCreateRequestFromInsertionPointsStillReplacesTheWholeXMLBody(t *testing.T) {
	history := xmlHistory("text/xml", soapEnvelope)
	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}
	xxePayload := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]><r>&x;</r>`

	body := requestBodyString(t, history, []InsertionPointBuilder{{Point: findXMLBodyPoints(points)[0], Payload: xxePayload}})

	if body != xxePayload {
		t.Errorf("the whole-body XXE surface must still replace the document, got: %s", body)
	}
}
