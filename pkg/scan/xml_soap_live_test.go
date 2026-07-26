package scan

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
)

const soapSQLiEndpoint = "http://127.0.0.1:9094/soap/sqli"

const soapGetUserRequest = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">` +
	`<soap:Body>` +
	`<tns:GetUser xmlns:tns="http://testbed.local/soap/sqli">` +
	`<tns:username>admin</tns:username>` +
	`</tns:GetUser>` +
	`</soap:Body>` +
	`</soap:Envelope>`

// TestSOAPElementInjectionReachesTheSQLSinkLive proves end to end that a payload placed
// in a per-element XML insertion point survives the SOAP envelope parse and reaches the
// backend SQL sink. Replacing the whole body -- the only surface that existed before --
// cannot do this: the envelope is destroyed and the service returns a parse fault.
//
//	SUKYAN_SOAP_LIVE=1 GOWORK=off go test ./pkg/scan/ -run TestSOAPElementInjectionReachesTheSQLSinkLive -v
func TestSOAPElementInjectionReachesTheSQLSinkLive(t *testing.T) {
	if os.Getenv("SUKYAN_SOAP_LIVE") == "" {
		t.Skip("set SUKYAN_SOAP_LIVE=1 (needs the soap testbed on :9094) to run")
	}

	history := &db.History{
		URL:                soapSQLiEndpoint,
		Method:             "POST",
		RequestContentType: "text/xml; charset=utf-8",
		RawRequest: []byte("POST /soap/sqli HTTP/1.1\r\nHost: 127.0.0.1:9094\r\n" +
			"Content-Type: text/xml; charset=utf-8\r\nSOAPAction: GetUser\r\n\r\n" + soapGetUserRequest),
	}

	points, err := GetInsertionPoints(history, []string{"xml"})
	if err != nil {
		t.Fatal(err)
	}
	target := findPoint(t, elementPoints(points), "username")

	baseline := sendSOAP(t, history, target, "admin")
	injected := sendSOAP(t, history, target, `admin' OR '1'='1`)

	if !strings.Contains(baseline, `"username": "admin"`) {
		t.Fatalf("the unmodified request did not reach the service: %s", baseline)
	}
	if strings.Contains(baseline, `"username": "user"`) {
		t.Fatalf("baseline already returned every user, the oracle is meaningless: %s", baseline)
	}
	if !strings.Contains(injected, `"username": "user"`) {
		t.Errorf("payload did not reach the SQL sink; response was: %s", injected)
	}
}

func sendSOAP(t *testing.T, history *db.History, point InsertionPoint, payload string) string {
	t.Helper()

	req, err := CreateRequestFromInsertionPoints(history, []InsertionPointBuilder{{Point: point, Payload: payload}})
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	return string(body)
}
