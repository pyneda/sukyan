package scan

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
)

func wsXMLPoints(t *testing.T, payload string) []InsertionPoint {
	t.Helper()
	points, err := GetWebSocketMessageInsertionPoints(&db.WebSocketMessage{PayloadData: payload, Opcode: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return points
}

func wsPointsOfType(points []InsertionPoint, kind InsertionPointType) []InsertionPoint {
	var matches []InsertionPoint
	for _, p := range points {
		if p.Type == kind {
			matches = append(matches, p)
		}
	}
	return matches
}

// Before the shared codec the WebSocket extractor emitted tag names, namespaces and
// processing instructions but never a single element value, so no XML message
// parameter was reachable.
func TestWebSocketXMLInsertionPointsIncludeLeafElementValues(t *testing.T) {
	points := wsXMLPoints(t, `<msg><cmd>ls</cmd><user>bob</user></msg>`)

	elements := wsPointsOfType(points, InsertionPointTypeWSXMLElement)
	byName := map[string]string{}
	for _, p := range elements {
		byName[p.Name] = p.Value
	}
	if byName["cmd"] != "ls" {
		t.Errorf("expected a cmd point valued %q, got points %v", "ls", byName)
	}
	if byName["user"] != "bob" {
		t.Errorf("expected a user point valued %q, got points %v", "bob", byName)
	}
}

func TestWebSocketXMLInsertionPointsKeepTheWholeDocumentPoint(t *testing.T) {
	payload := `<msg><cmd>ls</cmd></msg>`
	points := wsXMLPoints(t, payload)

	for _, p := range points {
		if p.Type == InsertionPointTypeWSXMLElement && p.Name == "document" {
			if p.Value != payload || p.ValueType != lib.TypeXML {
				t.Errorf("unexpected document point: %+v", p)
			}
			if p.Span.Valid {
				t.Error("the whole-document point must not carry a span")
			}
			return
		}
	}
	t.Errorf("expected the whole-document XXE point to survive, got %v", xmlPointNames(points))
}

func TestWebSocketXMLInsertionPointsDropTagAndNamespacePoints(t *testing.T) {
	points := wsXMLPoints(t, `<ns:msg xmlns:ns="http://example.com"><cmd>ls</cmd></ns:msg>`)

	for _, p := range points {
		switch p.Type {
		case "ws_xml_tag", "ws_xml_namespace", "ws_xml_ns_prefix", "ws_xml_processing":
			t.Errorf("low-yield point type %q should no longer be emitted (%q)", p.Type, p.Name)
		}
	}
}

func TestWebSocketXMLInsertionPointsKeepAttributeValues(t *testing.T) {
	points := wsXMLPoints(t, `<msg id="7"><cmd>ls</cmd></msg>`)

	attrs := wsPointsOfType(points, InsertionPointTypeWSXMLAttribute)
	if len(attrs) != 1 || attrs[0].Name != "id" || attrs[0].Value != "7" {
		t.Errorf("expected one id attribute point valued 7, got %+v", attrs)
	}
}

func TestCreateModifiedWebSocketMessageInjectsIntoALeafElement(t *testing.T) {
	payload := `<msg><cmd>ls</cmd><user>bob</user></msg>`
	original := &db.WebSocketMessage{PayloadData: payload, Opcode: 1}
	points := wsXMLPoints(t, payload)
	target := findPoint(t, wsPointsOfType(points, InsertionPointTypeWSXMLElement), "cmd")

	modified, err := CreateModifiedWebSocketMessage(original, target, "; id")
	if err != nil {
		t.Fatal(err)
	}

	if want := `<msg><cmd>; id</cmd><user>bob</user></msg>`; modified.PayloadData != want {
		t.Errorf("expected %q, got %q", want, modified.PayloadData)
	}
}

func TestCreateModifiedWebSocketMessageReplacesTheWholeDocument(t *testing.T) {
	payload := `<msg><cmd>ls</cmd></msg>`
	original := &db.WebSocketMessage{PayloadData: payload, Opcode: 1}
	document := findPoint(t, wsPointsOfType(wsXMLPoints(t, payload), InsertionPointTypeWSXMLElement), "document")

	modified, err := CreateModifiedWebSocketMessage(original, document, `<!DOCTYPE x><x/>`)
	if err != nil {
		t.Fatal(err)
	}

	if modified.PayloadData != `<!DOCTYPE x><x/>` {
		t.Errorf("the document point must replace the whole message, got %q", modified.PayloadData)
	}
}

func TestCreateModifiedWebSocketMessageTargetsOneRepeatedSibling(t *testing.T) {
	payload := `<list><item>a</item><item>b</item></list>`
	original := &db.WebSocketMessage{PayloadData: payload, Opcode: 1}
	target := findPoint(t, wsXMLPoints(t, payload), "/list/item[2]")

	modified, err := CreateModifiedWebSocketMessage(original, target, "X")
	if err != nil {
		t.Fatal(err)
	}

	if want := `<list><item>a</item><item>X</item></list>`; modified.PayloadData != want {
		t.Errorf("expected %q, got %q", want, modified.PayloadData)
	}
}

func TestCreateModifiedWebSocketMessageEscapesXMLPayloads(t *testing.T) {
	payload := `<msg><cmd>ls</cmd></msg>`
	original := &db.WebSocketMessage{PayloadData: payload, Opcode: 1}
	target := findPoint(t, wsPointsOfType(wsXMLPoints(t, payload), InsertionPointTypeWSXMLElement), "cmd")

	modified, err := CreateModifiedWebSocketMessage(original, target, `a & <b>`)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(modified.PayloadData, "<b>") {
		t.Errorf("payload markup must be escaped, got %q", modified.PayloadData)
	}
}
