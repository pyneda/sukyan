package scan

import (
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestCreateRequestFromURLParameter(t *testing.T) {
	history := &db.History{
		URL: "http://example.com",
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeParameter,
			Name: "param",
		},
		Payload: "value",
	}
	expectedURL := "http://example.com?param=value"

	result, err := createRequestFromURLParameter(history, builder)
	if err != nil {
		t.Fatal(err)
	}

	if result != expectedURL {
		t.Errorf("Expected URL: %s, Got: %s", expectedURL, result)
	}

}

func TestCreateRequestFromURLPath(t *testing.T) {
	history := &db.History{
		URL: "http://example.com/path1/path2",
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeURLPath,
			Name: "path1",
		},
		Payload: "modified_path1",
	}
	expectedURL := "http://example.com/modified_path1/path2"

	result, err := createRequestFromURLPath(history, builder)
	if err != nil {
		t.Fatal(err)
	}

	if result != expectedURL {
		t.Errorf("Expected URL: %s, Got: %s", expectedURL, result)
	}
}

func TestCreateRequestFromHeader(t *testing.T) {
	rawRequest := []byte("GET /path HTTP/1.1\r\nHeader1: value1\r\nHeader2: value2\r\n\r\nSome request body")

	history := &db.History{
		RawRequest: rawRequest,
	}

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeHeader,
			Name: "Header1",
		},
		Payload: "modified_value1",
	}

	expectedHeaders := http.Header{
		"Header1": []string{"modified_value1"},
		"Header2": []string{"value2"},
	}

	result, err := createRequestFromHeader(history, builder)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(result, expectedHeaders) {
		t.Errorf("Expected headers: %+v, Got: %+v", expectedHeaders, result)
	}
}

func TestCreateRequestFromCookie(t *testing.T) {
	rawRequest := []byte("GET /path HTTP/1.1\r\nCookie: cookie1=value1; cookie2=value2\r\nHost: example.com\r\n\r\nSome request body")

	history := &db.History{
		RawRequest: rawRequest,
	}

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeCookie,
			Name: "cookie1",
		},
		Payload: "modified_value",
	}

	expectedHeaders := http.Header{
		"Cookie": []string{"cookie1=modified_value; cookie2=value2"},
		"Host":   []string{"example.com"},
	}

	result, err := createRequestFromCookie(history, builder)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(result, expectedHeaders) {
		t.Errorf("Expected headers: %+v, Got: %+v", expectedHeaders, result)
	}
}

func TestCreateRequestFromBody_FormUrlEncoded(t *testing.T) {
	rawRequest := []byte("POST /path HTTP/1.1\r\nContent-Type: application/x-www-form-urlencoded\r\nHost: example.com\r\n\r\nparam1=value1&param2=value2")

	history := &db.History{
		RawRequest:         rawRequest,
		RequestContentType: "application/x-www-form-urlencoded",
	}

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeBody,
			Name: "param2",
		},
		Payload: "modified_value",
	}

	expectedBody := "param1=value1&param2=modified_value"
	expectedContentType := "application/x-www-form-urlencoded"

	result, contentType, err := createRequestFromBody(history, []InsertionPointBuilder{builder})
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := io.ReadAll(result)
	resultBody := string(bodyBytes)

	if resultBody != expectedBody {
		t.Errorf("Expected body: %s, Got: %s", expectedBody, resultBody)
	}

	if contentType != expectedContentType {
		t.Errorf("Expected Content-Type: %s, Got: %s", expectedContentType, contentType)
	}
}

func TestCreateRequestFromBody_JSON(t *testing.T) {
	rawRequest := []byte("POST /path HTTP/1.1\r\nContent-Type: application/json\r\nHost: example.com\r\n\r\n{\"param1\":\"value1\",\"param2\":\"value2\"}")

	history := &db.History{
		RawRequest:         rawRequest,
		RequestContentType: "application/json",
	}

	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: InsertionPointTypeBody,
			Name: "param2",
		},
		Payload: "modified_value",
	}

	expectedBody := `{"param1":"value1","param2":"modified_value"}`
	expectedContentType := "application/json"

	result, contentType, err := createRequestFromBody(history, []InsertionPointBuilder{builder})
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := io.ReadAll(result)
	resultBody := string(bodyBytes)

	if resultBody != expectedBody {
		t.Errorf("Expected body: %s, Got: %s", expectedBody, resultBody)
	}

	if contentType != expectedContentType {
		t.Errorf("Expected Content-Type: %s, Got: %s", expectedContentType, contentType)
	}
}
func TestCreateRequestFromInsertionPoints(t *testing.T) {
	// Create a raw HTTP request with JSON body
	rawRequest := []byte("POST http://example.com HTTP/1.1\r\nContent-Type: application/json\r\n\r\n{\"param1\":\"value1\",\"param2\":\"value2\"}")

	history := &db.History{
		URL:                "http://example.com",
		RawRequest:         rawRequest,
		RequestContentType: "application/json",
		Method:             "POST",
	}

	builders := []InsertionPointBuilder{
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeParameter,
				Name: "param1",
			},
			Payload: "modified_value1",
		},
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeBody,
				Name: "param1",
			},
			Payload: "modified_value1",
		},
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeHeader,
				Name: "Authorization",
			},
			Payload: "Bearer token",
		},
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeBody,
				Name: "param2",
			},
			Payload: "modified_value2",
		},
	}
	expectedURL := "http://example.com?param1=modified_value1"
	expectedHeaders := http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer token"},
	}
	expectedBody := `{"param1":"modified_value1","param2":"modified_value2"}`

	req, err := CreateRequestFromInsertionPoints(history, builders)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.String() != expectedURL {
		t.Errorf("Expected URL: %s, Got: %s", expectedURL, req.URL.String())
	}

	if !reflect.DeepEqual(req.Header, expectedHeaders) {
		t.Errorf("Expected headers: %+v, Got: %+v", expectedHeaders, req.Header)
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	resultBody := string(bodyBytes)

	if resultBody != expectedBody {
		t.Errorf("Expected body: %s, Got: %s", expectedBody, resultBody)
	}
}
func TestCreateRequestFromInsertionPoints_NoURLBuilder(t *testing.T) {
	rawRequest := []byte("POST http://example.com HTTP/1.1\r\nContent-Type: application/json\r\n\r\n{\"param1\":\"value1\",\"param2\":\"value2\"}")

	history := &db.History{
		URL:                "http://example.com",
		RawRequest:         rawRequest,
		RequestContentType: "application/json",
		Method:             "POST",
	}

	builders := []InsertionPointBuilder{
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeHeader,
				Name: "Authorization",
			},
			Payload: "Bearer token",
		},
		{
			Point: InsertionPoint{
				Type: InsertionPointTypeBody,
				Name: "param2",
			},
			Payload: "modified_value2",
		},
	}
	expectedURL := "http://example.com"
	expectedHeaders := http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer token"},
	}
	expectedBody := `{"param1":"value1","param2":"modified_value2"}`

	req, err := CreateRequestFromInsertionPoints(history, builders)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.String() != expectedURL {
		t.Errorf("Expected URL: %s, Got: %s", expectedURL, req.URL.String())
	}

	if !reflect.DeepEqual(req.Header, expectedHeaders) {
		t.Errorf("Expected headers: %+v, Got: %+v", expectedHeaders, req.Header)
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	resultBody := string(bodyBytes)

	if resultBody != expectedBody {
		t.Errorf("Expected body: %s, Got: %s", expectedBody, resultBody)
	}
}

func TestCreateRequestFromInsertionPoints_UnsupportedType(t *testing.T) {
	rawRequest := []byte("POST http://example.com HTTP/1.1\r\nContent-Type: application/json\r\n\r\n{\"param1\":\"value1\",\"param2\":\"value2\"}")

	history := &db.History{
		URL:                "http://example.com",
		RawRequest:         rawRequest,
		RequestContentType: "application/json",
		Method:             "POST",
	}

	builders := []InsertionPointBuilder{
		{
			Point: InsertionPoint{
				Type: "InvalidType",
				Name: "param1",
			},
			Payload: "modified_value1",
		},
	}
	expectedErrMsg := "unsupported insertion point type: InvalidType"

	_, err := CreateRequestFromInsertionPoints(history, builders)
	if err == nil || err.Error() != expectedErrMsg {
		t.Errorf("Expected error: %s, Got: %v", expectedErrMsg, err)
	}
}
