package scan

import (
	"reflect"
	"sort"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
)

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
