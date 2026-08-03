package report

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func sampleDBIssues() []*db.Issue {
	hostile := append([]byte("HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n"), 0x00, 0xff, 0xfe, 0x41)
	huge := append([]byte("HTTP/1.1 200 OK\r\n\r\n"), bytes.Repeat([]byte("A"), 300_000)...)

	return []*db.Issue{
		{
			BaseModel:   db.BaseModel{ID: 1},
			Code:        "sql_injection",
			Title:       "SQL Injection",
			Description: "The application concatenates untrusted input into a SQL statement.",
			Remediation: "Use parameterised queries.",
			URL:         "https://api.acme.test/v1/users?id=1",
			HTTPMethod:  "GET",
			StatusCode:  500,
			Severity:    "Critical",
			Confidence:  95,
			Cwe:         89,
			References:  []string{"https://owasp.org/www-community/attacks/SQL_Injection"},
			Payload:     "' OR 1=1--",
			Request:     []byte("GET /v1/users?id=1%27%20OR%201%3D1-- HTTP/1.1\r\nHost: api.acme.test\r\n\r\n"),
			Response:    hostile,
		},
		{
			BaseModel:   db.BaseModel{ID: 2},
			Code:        "reflected_input",
			Title:       "Reflected Input in Response",
			Description: "Untrusted input is reflected into the response body.",
			Remediation: "Encode output for its context.",
			URL:         "https://www.acme.test/search",
			HTTPMethod:  "GET",
			StatusCode:  200,
			Severity:    "High",
			Confidence:  80,
			Cwe:         79,
			Request:     []byte("GET /search?q=test HTTP/1.1\r\nHost: www.acme.test\r\n\r\n"),
			Response:    huge,
		},
		{
			BaseModel:   db.BaseModel{ID: 3},
			Code:        "reflected_input",
			Title:       "Reflected Input in Response",
			Description: "Untrusted input is reflected into the response body.",
			Remediation: "Encode output for its context.",
			URL:         "https://static.acme.test/search",
			HTTPMethod:  "POST",
			StatusCode:  200,
			Severity:    "High",
			Confidence:  60,
			Request:     []byte("POST /search HTTP/1.1\r\nHost: static.acme.test\r\n\r\nq=test"),
			Response:    []byte("HTTP/1.1 200 OK\r\n\r\nok"),
		},
		{
			BaseModel:   db.BaseModel{ID: 4},
			Code:        "cache_control_header",
			Title:       "Cache Control Header Misconfiguration",
			Description: "The response can be cached.",
			Remediation: "Set Cache-Control: no-store.",
			URL:         "https://www.acme.test/profile",
			HTTPMethod:  "GET",
			StatusCode:  200,
			Severity:    "Low",
			Confidence:  100,
			Request:     []byte("GET /profile HTTP/1.1\r\nHost: www.acme.test\r\n\r\n"),
			Response:    []byte("HTTP/1.1 200 OK\r\nCache-Control: public\r\n\r\n"),
		},
	}
}

func pdfOptions(issues []*db.Issue) ReportOptions {
	return ReportOptions{
		WorkspaceID: 7,
		Title:       "Acme Engagement",
		Format:      ReportFormatPDF,
		Issues:      issues,
		GeneratedAt: fixedTime(),
	}
}

func TestGeneratePDFReportIsDeterministic(t *testing.T) {
	var a, b bytes.Buffer
	require.NoError(t, generatePDFReport(pdfOptions(sampleDBIssues()), &a))
	require.NoError(t, generatePDFReport(pdfOptions(sampleDBIssues()), &b))

	require.True(t, bytes.Equal(a.Bytes(), b.Bytes()),
		"same input must produce byte-identical output (got %d vs %d bytes)", a.Len(), b.Len())
}

func TestGenerateReportDispatchesPDF(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, GenerateReport(pdfOptions(sampleDBIssues()), &buf))
	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}

func TestGeneratePDFReportWithNoIssues(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, generatePDFReport(ReportOptions{Title: "Empty", GeneratedAt: fixedTime()}, &buf))
	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}

func TestGeneratePDFReportSurvivesHostileEvidence(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, generatePDFReport(pdfOptions(sampleDBIssues()), &buf))

	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
	require.True(t, bytes.Contains(buf.Bytes(), []byte("%%EOF")), "document must be terminated")
}

func TestGeneratePDFReportCapsInstances(t *testing.T) {
	var issues []*db.Issue
	for i := 0; i < 400; i++ {
		issues = append(issues, &db.Issue{
			BaseModel: db.BaseModel{ID: uint(i + 1)}, Code: "cache_control_header",
			Title: "Cache Control Header Misconfiguration", Severity: "Low", Confidence: 100,
			URL: fmt.Sprintf("https://www.acme.test/page/%d", i), HTTPMethod: "GET", StatusCode: 200,
			Request: []byte("GET / HTTP/1.1\r\n\r\n"), Response: []byte("HTTP/1.1 200 OK\r\n\r\n"),
		})
	}

	opts := pdfOptions(issues)
	opts.MaxInstances = 10

	var buf bytes.Buffer
	require.NoError(t, generatePDFReport(opts, &buf))
	require.True(t, bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")))
}

func TestResolveLimitContract(t *testing.T) {
	require.Equal(t, 50, resolveLimit(0, 50), "zero takes the default")
	require.Equal(t, 0, resolveLimit(-1, 50), "negative means unlimited")
	require.Equal(t, 7, resolveLimit(7, 50), "positive is used as given")
}

func TestPDFReportEmbedsMetadataAndOutline(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, generatePDFReport(pdfOptions(sampleDBIssues()), &buf))

	require.True(t, bytes.Contains(buf.Bytes(), []byte("/Outlines")), "reader navigation pane needs an outline")
	require.True(t, bytes.Contains(buf.Bytes(), []byte("/Title")), "document metadata must be set")
}

func TestHTMLReportStaysReproducible(t *testing.T) {
	opts := pdfOptions(sampleDBIssues())
	opts.Format = ReportFormatHTML

	var a, b bytes.Buffer
	require.NoError(t, GenerateReport(opts, &a))
	require.NoError(t, GenerateReport(opts, &b))

	require.Equal(t, a.String(), b.String(), "the html format must remain byte-stable")
}

func TestWebSocketAndInteractionEvidenceRender(t *testing.T) {
	issue := &ReportIssue{
		Code: "ws", Title: "WebSocket Issue", Severity: "Medium", Confidence: 70,
		URL: "wss://api.acme.test/socket", HTTPMethod: "GET", StatusCode: 101,
		Interactions: []*ReportInteraction{{
			Protocol: "dns", RemoteAddress: "203.0.113.7", Timestamp: "2026-08-03 10:00:00",
			FullID:     "abc.oast.site",
			RawRequest: base64.StdEncoding.EncodeToString([]byte("query abc.oast.site")),
			Cause: &ReportInteractionCause{
				TestName: "Blind SQLi via DNS", Payload: "' OR 1=1--", InsertionPoint: "parameter:id",
			},
		}},
		WebSocketConnection: &ReportWebSocketConnection{
			URL: "wss://api.acme.test/socket", StatusCode: 101, StatusText: "Switching Protocols",
			Messages: []*ReportWebSocketMessage{
				{Direction: "sent", Timestamp: "2026-08-03 10:00:01", PayloadData: "{\"op\":\"ping\"}"},
				{Direction: "receive", Timestamp: "2026-08-03 10:00:02", PayloadData: string([]byte{0x00, 0xff, 0x41}), IsBinary: true},
			},
		},
	}

	findings := buildPDFFindings(groupIssuesByType([]*ReportIssue{issue}), 0)

	d, err := newPDFDoc(ReportOptions{Title: "T", GeneratedAt: fixedTime()})
	require.NoError(t, err)
	d.startBody()
	d.renderFindings(findings, defaultMaxEvidenceBytes)

	require.NoError(t, d.finish())
	require.NoError(t, d.pdf.Error())
}
