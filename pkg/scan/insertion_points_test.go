package scan

import (
	"encoding/json"
	"net/http"
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
	headerData := http.Header{
		"Header1": []string{"1"},
		"Header2": []string{"value2"},
	}
	jsonHeaderData, _ := json.Marshal(headerData)
	history := &db.History{
		RequestHeaders: jsonHeaderData,
	}
	expected := []InsertionPoint{
		{Type: InsertionPointTypeHeader, Name: "Header1", Value: "1", OriginalData: headerData["Header1"][0], ValueType: lib.TypeInt},
		{Type: InsertionPointTypeHeader, Name: "Header2", Value: "value2", OriginalData: headerData["Header2"][0], ValueType: lib.TypeString},
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
	headerData := http.Header{
		"Cookie": []string{"cookie1=value1; cookie2=value2; sessionid=U2Vzc2lvbkNvb2tpZT1zYW1wbGUxMjM0NTY3OA=="},
	}
	jsonHeaderData, _ := json.Marshal(headerData)
	history := &db.History{
		RequestHeaders: jsonHeaderData,
	}
	expected := []InsertionPoint{
		{Type: InsertionPointTypeCookie, Name: "cookie1", Value: "value1", OriginalData: headerData["Cookie"][0], ValueType: lib.TypeString},
		{Type: InsertionPointTypeCookie, Name: "cookie2", Value: "value2", OriginalData: headerData["Cookie"][0], ValueType: lib.TypeString},
		{Type: InsertionPointTypeCookie, Name: "sessionid", Value: "U2Vzc2lvbkNvb2tpZT1zYW1wbGUxMjM0NTY3OA==", OriginalData: headerData["Cookie"][0], ValueType: lib.TypeBase64},
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
	history := &db.History{
		RequestBody:        []byte("param1=value1&param2=value2"),
		RequestContentType: "application/x-www-form-urlencoded",
	}
	behaviour := InsertionPointBehaviour{
		IsReflected: false,
		IsDynamic:   false,
	}
	expected := []InsertionPoint{
		{Type: InsertionPointTypeBody, Name: "param1", Value: "value1", OriginalData: string(history.RequestBody), ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeBody, Name: "param2", Value: "value2", OriginalData: string(history.RequestBody), ValueType: lib.TypeString, Behaviour: behaviour},
		{Type: InsertionPointTypeFullBody, Name: "fullbody", Value: "param1=value1&param2=value2", OriginalData: string(history.RequestBody), ValueType: lib.TypeString, Behaviour: behaviour},
	}
	result, err := GetInsertionPoints(history, []string{"body"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %+v, got %+v", expected, result)
	}
}
