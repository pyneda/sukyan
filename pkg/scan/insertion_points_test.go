package scan

import (
	"io"
	"reflect"
	"sort"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
)

const xmlLaunchName = "xml"

// findXMLBodyPoints returns the whole-body insertion points produced for an XML body.
func findXMLBodyPoints(points []InsertionPoint) []InsertionPoint {
	var matches []InsertionPoint
	for _, p := range points {
		if p.Type == InsertionPointTypeFullBody && p.ValueType == lib.TypeXML {
			matches = append(matches, p)
		}
	}
	return matches
}

func TestHandleBodyParametersXML(t *testing.T) {
	xmlBody := `<?xml version="1.0"?><invoice><total>42</total></invoice>`

	cases := []struct {
		name        string
		contentType string
	}{
		{"application/xml", "application/xml"},
		{"text/xml", "text/xml"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := &db.History{
				RawRequest:         []byte("POST /path HTTP/1.1\r\nContent-Type: " + tc.contentType + "\r\n\r\n" + xmlBody),
				RequestContentType: tc.contentType,
			}

			points, err := GetInsertionPoints(history, []string{"body"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			xmlPoints := findXMLBodyPoints(points)
			if len(xmlPoints) != 1 {
				t.Fatalf("expected exactly one XML whole-body insertion point, got %d (all points: %+v)", len(xmlPoints), points)
			}
			p := xmlPoints[0]
			if p.Name != xmlLaunchName {
				t.Errorf("expected XML insertion point name %q so xxe.yaml launches, got %q", xmlLaunchName, p.Name)
			}
			if p.Value != xmlBody || p.OriginalData != xmlBody {
				t.Errorf("expected whole-body value/original, got value=%q original=%q", p.Value, p.OriginalData)
			}

			fullBodyCount := 0
			for _, ip := range points {
				if ip.Type == InsertionPointTypeFullBody {
					fullBodyCount++
				}
			}
			if fullBodyCount != 1 {
				t.Errorf("expected exactly one full-body insertion point for XML, got %d", fullBodyCount)
			}
		})
	}
}

// The content-type column is empty for crawler-discovered POSTs; the content type
// must be recovered from the RawRequest headers so the XML point is still produced.
func TestHandleBodyParametersXMLContentTypeFallback(t *testing.T) {
	xmlBody := `<?xml version="1.0"?><invoice><total>42</total></invoice>`
	history := &db.History{
		RawRequest:         []byte("POST /path HTTP/1.1\r\ncontent-type: application/xml\r\n\r\n" + xmlBody),
		RequestContentType: "",
	}

	points, err := GetInsertionPoints(history, []string{"body"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	xmlPoints := findXMLBodyPoints(points)
	if len(xmlPoints) != 1 {
		t.Fatalf("expected one XML whole-body insertion point from content-type fallback, got %d (all: %+v)", len(xmlPoints), points)
	}
	if xmlPoints[0].Name != xmlLaunchName {
		t.Errorf("expected name %q, got %q", xmlLaunchName, xmlPoints[0].Name)
	}
}

// A non-XML body must be entirely unchanged: no XML point, no duplicate fullbody.
func TestHandleBodyParametersNonXMLUnchanged(t *testing.T) {
	behaviour := InsertionPointBehaviour{}

	jsonBody := `{"a":"1","b":"2"}`
	jsonHistory := &db.History{
		RawRequest:         []byte("POST /path HTTP/1.1\r\nContent-Type: application/json\r\n\r\n" + jsonBody),
		RequestContentType: "application/json",
	}

	jsonPoints, err := GetInsertionPoints(jsonHistory, []string{"body"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findXMLBodyPoints(jsonPoints)) != 0 {
		t.Errorf("non-XML body must not produce XML insertion points")
	}
	jsonFullBody := 0
	for _, p := range jsonPoints {
		if p.Type == InsertionPointTypeFullBody {
			jsonFullBody++
			if p.Name != "fullbody" {
				t.Errorf("non-XML full body point renamed to %q", p.Name)
			}
		}
	}
	if jsonFullBody != 1 {
		t.Errorf("expected exactly one fullbody point for JSON, got %d", jsonFullBody)
	}

	formBody := "param1=value1&param2=value2"
	formHistory := &db.History{
		RawRequest:         []byte("POST /path HTTP/1.1\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\n" + formBody),
		RequestContentType: "application/x-www-form-urlencoded",
	}
	expected := []InsertionPoint{
		{Type: InsertionPointTypeBody, Name: "param1", Value: "value1", OriginalData: formBody, ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeBody, Name: "param2", Value: "value2", OriginalData: formBody, ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeFullBody, Name: "fullbody", Value: formBody, OriginalData: formBody, ValueType: lib.TypeString, Behaviour: behaviour},
	}
	formPoints, err := GetInsertionPoints(formHistory, []string{"body"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(formPoints) != len(expected) {
		t.Fatalf("form body: expected %d points, got %d (%+v)", len(expected), len(formPoints), formPoints)
	}
	expectedMap := make(map[string]InsertionPoint)
	for _, p := range expected {
		expectedMap[string(p.Type)+":"+p.Name] = p
	}
	for _, p := range formPoints {
		exp, ok := expectedMap[string(p.Type)+":"+p.Name]
		if !ok {
			t.Errorf("unexpected form point: %+v", p)
			continue
		}
		if !reflect.DeepEqual(p, exp) {
			t.Errorf("form point mismatch: expected %+v got %+v", exp, p)
		}
	}
}

// The XML whole-body point must yield a request whose body is exactly the payload.
func TestCreateRequestFromXMLFullBody(t *testing.T) {
	xmlBody := `<?xml version="1.0"?><invoice><total>42</total></invoice>`
	history := &db.History{
		URL:                "http://example.com/path",
		Method:             "POST",
		RawRequest:         []byte("POST /path HTTP/1.1\r\nContent-Type: application/xml\r\n\r\n" + xmlBody),
		RequestContentType: "application/xml",
	}

	points, err := GetInsertionPoints(history, []string{"body"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	xmlPoints := findXMLBodyPoints(points)
	if len(xmlPoints) != 1 {
		t.Fatalf("expected one XML whole-body point, got %d", len(xmlPoints))
	}

	payload := `<?xml version="1.0"?><!DOCTYPE data [<!ENTITY x SYSTEM "file:///etc/passwd">]><data>&x;</data>`
	req, err := CreateRequestFromInsertionPoints(history, []InsertionPointBuilder{{Point: xmlPoints[0], Payload: payload}})
	if err != nil {
		t.Fatalf("unexpected error building request: %v", err)
	}
	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	if string(got) != payload {
		t.Errorf("expected request body to be the whole payload.\nexpected: %q\ngot:      %q", payload, string(got))
	}
}

func TestHandleURLParameters(t *testing.T) {
	history := &db.History{
		URL: "http://example.com/path?param1=value1&param2=value2",
	}
	expected := []InsertionPoint{
		{Type: InsertionPointTypeParameter, Name: "param1", Value: "value1", OriginalData: history.URL, ValueType: lib.TypeString},
		{Type: InsertionPointTypeParameter, Name: "param2", Value: "value2", OriginalData: history.URL, ValueType: lib.TypeString},
	}

	result, err := GetInsertionPoints(history, []string{"parameters"})
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != len(expected) {
		t.Errorf("Expected %d URL parameter(s), got %d", len(expected), len(result))
	}

	// Sort the URL parameters by name for consistent order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	for i, expectedParam := range expected {
		if result[i].Type != expectedParam.Type || result[i].Name != expectedParam.Name ||
			result[i].Value != expectedParam.Value || result[i].OriginalData != expectedParam.OriginalData {
			t.Errorf("Expected URL parameter %+v at index %d, got %+v", expectedParam, i, result[i])
		}
	}
}

func TestHandleHeaders(t *testing.T) {
	rawRequest := []byte("GET /path HTTP/1.1\r\nHeader1: 1\r\nHeader2: value2\r\n\r\n")

	history := &db.History{
		RawRequest: rawRequest,
	}

	expected := []InsertionPoint{
		{Type: InsertionPointTypeHeader, Name: "Header1", Value: "1", OriginalData: "1", ValueType: lib.TypeInt},
		{Type: InsertionPointTypeHeader, Name: "Header2", Value: "value2", OriginalData: "value2", ValueType: lib.TypeString},
	}

	result, err := GetInsertionPoints(history, []string{"headers"})
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != len(expected) {
		t.Errorf("Expected %d header(s), got %d", len(expected), len(result))
	}

	// Sort the headers by name for consistent order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	for i, expectedHeader := range expected {
		if result[i].Type != expectedHeader.Type || result[i].Name != expectedHeader.Name ||
			result[i].Value != expectedHeader.Value || result[i].OriginalData != expectedHeader.OriginalData {
			t.Errorf("Expected header %+v at index %d, got %+v", expectedHeader, i, result[i])
		}
	}
}

func TestHandleCookies(t *testing.T) {
	rawRequest := []byte("GET /path HTTP/1.1\r\nCookie: cookie1=value1; cookie2=value2; sessionid=U2Vzc2lvbkNvb2tpZT1zYW1wbGUxMjM0NTY3OA==\r\n\r\n")

	history := &db.History{
		RawRequest: rawRequest,
	}

	cookieStr := "cookie1=value1; cookie2=value2; sessionid=U2Vzc2lvbkNvb2tpZT1zYW1wbGUxMjM0NTY3OA=="

	expected := []InsertionPoint{
		{Type: InsertionPointTypeCookie, Name: "cookie1", Value: "value1", OriginalData: cookieStr, ValueType: lib.TypeString},
		{Type: InsertionPointTypeCookie, Name: "cookie2", Value: "value2", OriginalData: cookieStr, ValueType: lib.TypeString},
		{Type: InsertionPointTypeCookie, Name: "sessionid", Value: "U2Vzc2lvbkNvb2tpZT1zYW1wbGUxMjM0NTY3OA==", OriginalData: cookieStr, ValueType: lib.TypeBase64},
	}

	result, err := GetInsertionPoints(history, []string{"cookies"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %+v, got %+v", expected, result)
	}
}

func TestHandleBodyParameters(t *testing.T) {
	rawRequest := []byte("POST /path HTTP/1.1\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nparam1=value1&param2=value2")

	history := &db.History{
		RawRequest:         rawRequest,
		RequestContentType: "application/x-www-form-urlencoded",
	}

	behaviour := InsertionPointBehaviour{
		IsReflected: false,
		IsDynamic:   false,
	}

	bodyContent := "param1=value1&param2=value2"

	expected := []InsertionPoint{
		{Type: InsertionPointTypeBody, Name: "param1", Value: "value1", OriginalData: bodyContent, ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeBody, Name: "param2", Value: "value2", OriginalData: bodyContent, ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeFullBody, Name: "fullbody", Value: bodyContent, OriginalData: bodyContent, ValueType: lib.TypeString, Behaviour: behaviour},
	}

	result, err := GetInsertionPoints(history, []string{"body"})
	if err != nil {
		t.Fatal(err)
	}

	// Check that we have the expected number of insertion points
	if len(result) != len(expected) {
		t.Errorf("Expected %d insertion points, got %d", len(expected), len(result))
		return
	}

	// Convert expected to a map for easier comparison
	expectedMap := make(map[string]InsertionPoint)
	for _, point := range expected {
		key := string(point.Type) + ":" + point.Name
		expectedMap[key] = point
	}

	// Check that all expected points are present
	for _, point := range result {
		key := string(point.Type) + ":" + point.Name
		expectedPoint, exists := expectedMap[key]
		if !exists {
			t.Errorf("Unexpected insertion point: %+v", point)
			continue
		}
		if !reflect.DeepEqual(point, expectedPoint) {
			t.Errorf("Insertion point mismatch for %s. Expected %+v, got %+v", key, expectedPoint, point)
		}
	}
}
