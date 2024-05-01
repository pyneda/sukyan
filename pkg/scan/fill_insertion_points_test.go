package scan

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/pyneda/sukyan/db"
	"gorm.io/datatypes"
)

func TestCreateRequestFromURLParameter(t *testing.T) {
	history := &db.History{
		URL: "http://example.com",
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: "Parameter",
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
			Type: "Urlpath",
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
	headerData := http.Header{
		"Header1": []string{"value1"},
		"Header2": []string{"value2"},
	}
	jsonHeaderData, _ := json.Marshal(headerData)
	history := &db.History{
		RequestHeaders: datatypes.JSON(jsonHeaderData),
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: "Header",
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
	headerData := http.Header{
		"Cookie": []string{"cookie1=value1; cookie2=value2"},
	}
	jsonHeaderData, _ := json.Marshal(headerData)
	history := &db.History{
		RequestHeaders: datatypes.JSON(jsonHeaderData),
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: "Cookie",
			Name: "cookie1",
		},
		Payload: "modified_value",
	}
	expectedHeaders := http.Header{
		"Cookie": []string{"cookie1=modified_value; cookie2=value2"},
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
	history := &db.History{
		RequestContentType: "application/x-www-form-urlencoded",
		RequestBody:        []byte("param1=value1&param2=value2"),
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: "Body",
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
	history := &db.History{
		RequestContentType: "application/json",
		RequestBody:        []byte(`{"param1":"value1","param2":"value2"}`),
	}
	builder := InsertionPointBuilder{
		Point: InsertionPoint{
			Type: "Body",
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
	history := &db.History{
		URL:                  "http://example.com",
		RequestHeaders:       datatypes.JSON(json.RawMessage(`{"Content-Type": ["application/json"]}`)),
		RequestContentType:   "application/json",
		RequestBody:          []byte(`{"param1":"value1","param2":"value2"}`),
		RequestContentLength: 32,
		Method:               "POST",
	}
	builders := []InsertionPointBuilder{
		{
			Point: InsertionPoint{
				Type: "Parameter",
				Name: "param1",
			},
			Payload: "modified_value1",
		},
		{
			Point: InsertionPoint{
				Type: "Body",
				Name: "param1",
			},
			Payload: "modified_value1",
		},
		{
			Point: InsertionPoint{
				Type: "Header",
				Name: "Authorization",
			},
			Payload: "Bearer token",
		},
		{
			Point: InsertionPoint{
				Type: "Body",
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
	history := &db.History{
		URL:                "http://example.com",
		RequestHeaders:     datatypes.JSON(json.RawMessage(`{"Content-Type": ["application/json"]}`)),
		RequestContentType: "application/json",
		RequestBody:        []byte(`{"param1":"value1","param2":"value2"}`),
		Method:             "POST",
	}
	builders := []InsertionPointBuilder{
		{
			Point: InsertionPoint{
				Type: "Header",
				Name: "Authorization",
			},
			Payload: "Bearer token",
		},
		{
			Point: InsertionPoint{
				Type: "Body",
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
	history := &db.History{
		URL:                "http://example.com",
		RequestHeaders:     datatypes.JSON(json.RawMessage(`{"Content-Type": ["application/json"]}`)),
		RequestContentType: "application/json",
		RequestBody:        []byte(`{"param1":"value1","param2":"value2"}`),
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
	expectedErrMsg := "unsupported insertion point type"

	_, err := CreateRequestFromInsertionPoints(history, builders)
	if err == nil || err.Error() != expectedErrMsg {
		t.Errorf("Expected error: %s, Got: %v", expectedErrMsg, err)
	}
}

// func TestCreateRequestFromInsertionPoints_InvalidContentType(t *testing.T) {
// 	history := &db.History{
// 		URL:                "http://example.com",
// 		RequestHeaders:     datatypes.JSON(json.RawMessage(`{"Content-Type": ["application/aaa"]}`)),
// 		RequestContentType: "application/aaaa",
// 		RequestBody:        []byte(`{"param1":"value1","param2":"value2"}`),
// 		Method:             "POST",
// 	}
// 	builders := []InsertionPointBuilder{
// 		{
// 			Point: InsertionPoint{
// 				Type: "Body",
// 				Name: "param1",
// 			},
// 			Payload: "modified_value1",
// 		},
// 	}
// 	expectedErrMsg := "unsupported Content-Type for body"

// 	_, err := CreateRequestFromInsertionPoints(history, builders)
// 	if err == nil || err.Error() != expectedErrMsg {
// 		t.Errorf("Expected error: %s, Got: %v", expectedErrMsg, err)
// 	}
// }
